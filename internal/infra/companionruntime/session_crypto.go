package companionruntime

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	challengeBytes      = 8 + 32 + 16 + 32
	clientProofBytes    = 8 + 16 + 32 + 32
	serverProofBytes    = 8 + 16 + 32
	frameHeaderBytes    = 4 + 8
	maxCiphertext       = 128 * 1024
	maxInnerJSON        = 8 * 1024
	maxInnerPayload     = 96 * 1024
	defaultWriteTimeout = 2 * time.Second

	clientProofDomain = "tobari/credential-companion/client-proof/v1\x00"
	serverProofDomain = "tobari/credential-companion/server-proof/v1\x00"
	sessionSaltDomain = "tobari/credential-companion/session-salt/v1\x00"
	sessionKeyInfo    = "tobari/credential-companion/session-key/v1\x00"
)

var (
	challengeMagic     = []byte("TBC2CHAL")
	clientMagic        = []byte("TBC2CLNT")
	serverMagic        = []byte("TBC2SRVR")
	frameMagic         = []byte("TBC2FRM1")
	brokerDirection    = []byte("B2C1")
	companionDirection = []byte("C2B1")

	ErrProtocol = errors.New("credential companion protocol failed")
)

type encryptedSession struct {
	reader io.Reader
	writer io.Writer

	sessionID        [16]byte
	sendAEAD         cipher.AEAD
	receiveAEAD      cipher.AEAD
	sendDirection    []byte
	receiveDirection []byte
	sendSequence     uint64
	receiveSequence  uint64
	sendMu           sync.Mutex
	closeOnce        sync.Once
	closed           atomic.Bool
	writeTimeout     time.Duration
}

type writeDeadlineSetter interface {
	SetWriteDeadline(time.Time) error
}

func boundedUint32Length(length, maximum int) (uint32, bool) {
	if length < 0 || maximum < 0 || maximum > maxCiphertext || length > maximum {
		return 0, false
	}
	// #nosec G115 -- length is non-negative and bounded by maxCiphertext (131072).
	return uint32(length), true
}

func clientHandshake(
	source io.Reader,
	destination io.Writer,
	bootstrap *Bootstrap,
	entropy io.Reader,
) (*encryptedSession, error) {
	if source == nil || destination == nil || bootstrap == nil ||
		len(bootstrap.sessionKey) != sessionKeyBytes || entropy == nil {
		return nil, ErrProtocol
	}
	challenge := make([]byte, challengeBytes)
	if _, err := io.ReadFull(source, challenge); err != nil {
		clear(challenge)
		return nil, ErrProtocol
	}
	defer clear(challenge)
	if !bytes.Equal(challenge[:8], challengeMagic) {
		return nil, ErrProtocol
	}
	expectedEpoch, err := epochRaw(bootstrap.document.EpochID)
	if err != nil {
		return nil, ErrProtocol
	}
	defer clear(expectedEpoch)
	if !hmac.Equal(challenge[8:40], expectedEpoch) {
		return nil, ErrProtocol
	}
	brokerBoot := append([]byte(nil), challenge[40:56]...)
	brokerNonce := append([]byte(nil), challenge[56:88]...)
	defer clear(brokerBoot)
	defer clear(brokerNonce)
	instance := make([]byte, 16)
	companionNonce := make([]byte, 32)
	if _, err := io.ReadFull(entropy, instance); err != nil {
		clear(instance)
		clear(companionNonce)
		return nil, ErrProtocol
	}
	if _, err := io.ReadFull(entropy, companionNonce); err != nil {
		clear(instance)
		clear(companionNonce)
		return nil, ErrProtocol
	}
	defer clear(instance)
	defer clear(companionNonce)
	clientHeader := make([]byte, 0, 8+len(instance)+len(companionNonce))
	clientHeader = append(clientHeader, clientMagic...)
	clientHeader = append(clientHeader, instance...)
	clientHeader = append(clientHeader, companionNonce...)
	defer clear(clientHeader)
	clientMAC := computeHMAC(
		bootstrap.sessionKey,
		[]byte(clientProofDomain), challenge, clientHeader,
	)
	defer clear(clientMAC)
	clientProof := append(append([]byte(nil), clientHeader...), clientMAC...)
	if len(clientProof) != clientProofBytes || writeFull(destination, clientProof) != nil {
		clear(clientProof)
		return nil, ErrProtocol
	}
	clear(clientProof)
	serverProof := make([]byte, serverProofBytes)
	if _, err := io.ReadFull(source, serverProof); err != nil {
		clear(serverProof)
		return nil, ErrProtocol
	}
	defer clear(serverProof)
	if !bytes.Equal(serverProof[:8], serverMagic) {
		return nil, ErrProtocol
	}
	sessionID := append([]byte(nil), serverProof[8:24]...)
	defer clear(sessionID)
	serverHeader := append(append([]byte(nil), serverMagic...), sessionID...)
	defer clear(serverHeader)
	expectedServerMAC := computeHMAC(
		bootstrap.sessionKey,
		[]byte(serverProofDomain), challenge, clientHeader, clientMAC, serverHeader,
	)
	defer clear(expectedServerMAC)
	if !hmac.Equal(serverProof[24:], expectedServerMAC) {
		return nil, ErrProtocol
	}
	brokerKey, companionKey, err := deriveSessionKeys(
		bootstrap.sessionKey, expectedEpoch, brokerBoot, brokerNonce,
		instance, companionNonce, sessionID,
	)
	if err != nil {
		return nil, ErrProtocol
	}
	defer clear(brokerKey)
	defer clear(companionKey)
	receiveAEAD, err := newAEAD(brokerKey)
	if err != nil {
		return nil, ErrProtocol
	}
	sendAEAD, err := newAEAD(companionKey)
	if err != nil {
		return nil, ErrProtocol
	}
	session := &encryptedSession{
		reader: source, writer: destination,
		receiveAEAD: receiveAEAD, sendAEAD: sendAEAD,
		receiveDirection: brokerDirection, sendDirection: companionDirection,
	}
	copy(session.sessionID[:], sessionID)
	return session, nil
}

func deriveSessionKeys(
	epochKey, epoch, brokerBoot, brokerNonce, instance, companionNonce, sessionID []byte,
) ([]byte, []byte, error) {
	if len(epochKey) != 32 || len(epoch) != 32 || len(brokerBoot) != 16 ||
		len(brokerNonce) != 32 || len(instance) != 16 || len(companionNonce) != 32 || len(sessionID) != 16 {
		return nil, nil, ErrProtocol
	}
	saltInput := make([]byte, 0, len(sessionSaltDomain)+64)
	saltInput = append(saltInput, sessionSaltDomain...)
	saltInput = append(saltInput, brokerNonce...)
	saltInput = append(saltInput, companionNonce...)
	salt := sha256.Sum256(saltInput)
	clear(saltInput)
	baseInfo := make([]byte, 0, len(sessionKeyInfo)+32+16+16+16)
	baseInfo = append(baseInfo, sessionKeyInfo...)
	baseInfo = append(baseInfo, epoch...)
	baseInfo = append(baseInfo, brokerBoot...)
	baseInfo = append(baseInfo, instance...)
	baseInfo = append(baseInfo, sessionID...)
	defer clear(baseInfo)
	brokerInfo := append(append(append([]byte(nil), baseInfo...), 0), []byte("broker-to-companion")...)
	companionInfo := append(append(append([]byte(nil), baseInfo...), 0), []byte("companion-to-broker")...)
	defer clear(brokerInfo)
	defer clear(companionInfo)
	brokerKey, err := hkdf.Key(sha256.New, epochKey, salt[:], string(brokerInfo), 32)
	if err != nil {
		clear(salt[:])
		return nil, nil, ErrProtocol
	}
	companionKey, err := hkdf.Key(sha256.New, epochKey, salt[:], string(companionInfo), 32)
	clear(salt[:])
	if err != nil {
		clear(brokerKey)
		return nil, nil, ErrProtocol
	}
	return brokerKey, companionKey, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func computeHMAC(key []byte, chunks ...[]byte) []byte {
	mac := hmac.New(sha256.New, key)
	for _, chunk := range chunks {
		_, _ = mac.Write(chunk)
	}
	return mac.Sum(nil)
}

func (s *encryptedSession) send(document map[string]any, payload []byte) error {
	if s == nil || s.writer == nil || s.sendAEAD == nil || len(s.sendDirection) != 4 || s.closed.Load() {
		return ErrProtocol
	}
	plaintext, err := encodeInner(document, payload)
	if err != nil {
		return err
	}
	defer clear(plaintext)
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.closed.Load() {
		return ErrProtocol
	}
	deadlineWriter, ok := s.writer.(writeDeadlineSetter)
	if !ok {
		s.close()
		return ErrProtocol
	}
	writeTimeout := s.writeTimeout
	if writeTimeout <= 0 || writeTimeout > defaultWriteTimeout {
		writeTimeout = defaultWriteTimeout
	}
	if err := deadlineWriter.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		s.close()
		return ErrProtocol
	}
	defer func() { _ = deadlineWriter.SetWriteDeadline(time.Time{}) }()
	sequence := s.sendSequence
	if sequence == ^uint64(0) {
		return ErrProtocol
	}
	ciphertextLength := len(plaintext) + s.sendAEAD.Overhead()
	if ciphertextLength < 16 || ciphertextLength > maxCiphertext {
		return ErrProtocol
	}
	encodedCiphertextLength, ok := boundedUint32Length(ciphertextLength, maxCiphertext)
	if !ok {
		return ErrProtocol
	}
	nonce := frameNonce(s.sendDirection, sequence)
	aad := s.frameAAD(s.sendDirection, sequence, encodedCiphertextLength)
	ciphertext := s.sendAEAD.Seal(nil, nonce, plaintext, aad)
	defer clear(ciphertext)
	header := make([]byte, frameHeaderBytes)
	binary.BigEndian.PutUint32(header[:4], encodedCiphertextLength)
	binary.BigEndian.PutUint64(header[4:], sequence)
	if err := writeFull(s.writer, header); err != nil || writeFull(s.writer, ciphertext) != nil {
		clear(header)
		s.close()
		return ErrProtocol
	}
	clear(header)
	s.sendSequence++
	return nil
}

func (s *encryptedSession) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		if deadlineWriter, ok := s.writer.(writeDeadlineSetter); ok {
			_ = deadlineWriter.SetWriteDeadline(time.Now())
		}
		if closer, ok := s.writer.(io.Closer); ok {
			_ = closer.Close()
		}
		if closer, ok := s.reader.(io.Closer); ok {
			_ = closer.Close()
		}
	})
}

func (s *encryptedSession) receive() (map[string]json.RawMessage, []byte, error) {
	if s == nil || s.reader == nil || s.receiveAEAD == nil || len(s.receiveDirection) != 4 {
		return nil, nil, ErrProtocol
	}
	header := make([]byte, frameHeaderBytes)
	if _, err := io.ReadFull(s.reader, header); err != nil {
		clear(header)
		return nil, nil, ErrProtocol
	}
	encodedCiphertextLength := binary.BigEndian.Uint32(header[:4])
	ciphertextLength := int(encodedCiphertextLength)
	sequence := binary.BigEndian.Uint64(header[4:])
	clear(header)
	if ciphertextLength < 16 || ciphertextLength > maxCiphertext || sequence != s.receiveSequence || sequence == ^uint64(0) {
		return nil, nil, ErrProtocol
	}
	ciphertext := make([]byte, ciphertextLength)
	if _, err := io.ReadFull(s.reader, ciphertext); err != nil {
		clear(ciphertext)
		return nil, nil, ErrProtocol
	}
	nonce := frameNonce(s.receiveDirection, sequence)
	aad := s.frameAAD(s.receiveDirection, sequence, encodedCiphertextLength)
	plaintext, err := s.receiveAEAD.Open(nil, nonce, ciphertext, aad)
	clear(ciphertext)
	if err != nil {
		clear(plaintext)
		return nil, nil, ErrProtocol
	}
	s.receiveSequence++
	document, payload, err := decodeInner(plaintext)
	clear(plaintext)
	if err != nil {
		clear(payload)
		return nil, nil, err
	}
	return document, payload, nil
}

func frameNonce(direction []byte, sequence uint64) []byte {
	nonce := make([]byte, 12)
	copy(nonce, direction)
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

func (s *encryptedSession) frameAAD(direction []byte, sequence uint64, ciphertextLength uint32) []byte {
	aad := make([]byte, 0, 8+16+4+8+4)
	aad = append(aad, frameMagic...)
	aad = append(aad, s.sessionID[:]...)
	aad = append(aad, direction...)
	var encoded [12]byte
	binary.BigEndian.PutUint64(encoded[:8], sequence)
	binary.BigEndian.PutUint32(encoded[8:], ciphertextLength)
	aad = append(aad, encoded[:]...)
	return aad
}

func writeFull(destination io.Writer, payload []byte) error {
	for len(payload) != 0 {
		written, err := destination.Write(payload)
		if err != nil || written <= 0 {
			return fmt.Errorf("%w: write channel", ErrProtocol)
		}
		payload = payload[written:]
	}
	return nil
}
