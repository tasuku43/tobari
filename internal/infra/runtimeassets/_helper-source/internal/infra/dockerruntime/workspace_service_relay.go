package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

var workspaceServiceUnavailableResponse = func() []byte {
	body := "Workspace service is not available yet.\nStart the service, then reload.\n"
	return []byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain; charset=utf-8\r\nCache-Control: no-store\r\nConnection: close\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
}()

type serviceRequestMeta struct {
	method        string
	upgrade       bool
	upgradeResult chan bool
}

func validateServiceRequestHeader(header []byte, request *http.Request, authority string) error {
	if request == nil || request.Host != authority || (request.URL.IsAbs() && request.URL.Host != authority) {
		return errors.New("HTTP authority is not the exact exposure authority")
	}
	lines := bytes.Split(header, []byte("\r\n"))
	hosts, contentLengths, transferEncodings := 0, 0, 0
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			return errors.New("folded HTTP headers are unsupported")
		}
		name, _, found := bytes.Cut(line, []byte(":"))
		if !found {
			return errors.New("invalid HTTP header field")
		}
		switch strings.ToLower(string(name)) {
		case "host":
			hosts++
		case "content-length":
			contentLengths++
		case "transfer-encoding":
			transferEncodings++
		}
	}
	if hosts != 1 || contentLengths > 1 || transferEncodings > 1 || (contentLengths != 0 && transferEncodings != 0) {
		return errors.New("ambiguous HTTP authority or framing")
	}
	return nil
}

func readServiceHeader(reader *bufio.Reader) ([]byte, error) {
	var result []byte
	for len(result) <= workspaceServiceHeaderLimit {
		line, err := reader.ReadBytes('\n')
		result = append(result, line...)
		if err != nil {
			return nil, err
		}
		if len(result) > workspaceServiceHeaderLimit {
			return nil, errors.New("HTTP header exceeds limit")
		}
		if bytes.HasSuffix(result, []byte("\r\n\r\n")) {
			return result, nil
		}
	}
	return nil, errors.New("HTTP header exceeds limit")
}

func copyExactChunked(destination io.Writer, source *bufio.Reader) error {
	for {
		line, err := source.ReadBytes('\n')
		if err != nil || len(line) > 4096 || !bytes.HasSuffix(line, []byte("\r\n")) {
			return errors.New("invalid chunk framing")
		}
		if _, err = destination.Write(line); err != nil {
			return err
		}
		sizeText := strings.TrimSpace(strings.SplitN(string(line), ";", 2)[0])
		size, err := strconv.ParseUint(sizeText, 16, 63)
		if err != nil {
			return errors.New("invalid chunk size")
		}
		if size > 0 {
			if _, err = io.CopyN(destination, source, int64(size)+2); err != nil {
				return err
			}
			continue
		}
		for {
			trailer, err := source.ReadBytes('\n')
			if err != nil || len(trailer) > workspaceServiceHeaderLimit {
				return errors.New("invalid chunk trailer")
			}
			if _, err = destination.Write(trailer); err != nil {
				return err
			}
			if bytes.Equal(trailer, []byte("\r\n")) {
				return nil
			}
		}
	}
}

func requestHasChunked(request *http.Request) bool {
	for _, value := range request.TransferEncoding {
		if strings.EqualFold(value, "chunked") {
			return true
		}
	}
	return false
}

func copyServiceRequestBody(destination io.Writer, source *bufio.Reader, request *http.Request) error {
	if requestHasChunked(request) && request.ContentLength >= 0 {
		return errors.New("ambiguous HTTP request framing")
	}
	if requestHasChunked(request) {
		return copyExactChunked(destination, source)
	}
	if request.ContentLength > 0 {
		_, err := io.CopyN(destination, source, request.ContentLength)
		return err
	}
	return nil
}

func copyServiceResponseBody(destination io.Writer, source *bufio.Reader, response *http.Response, method string) (bool, error) {
	if method == http.MethodHead || response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotModified {
		return true, nil
	}
	for _, value := range response.TransferEncoding {
		if strings.EqualFold(value, "chunked") {
			return true, copyExactChunked(destination, source)
		}
	}
	if response.ContentLength >= 0 {
		_, err := io.CopyN(destination, source, response.ContentLength)
		return true, err
	}
	_, err := io.Copy(destination, source)
	return false, err
}

func (c *workspaceServiceController) openWorkspaceServiceStream(ctx context.Context, targetPort int) (io.WriteCloser, io.ReadCloser, <-chan error, error) {
	runner, ok := c.runtime.runner.(workspaceServiceControlRunner)
	if !ok {
		return nil, nil, nil, errors.New("service stream runtime is unavailable")
	}
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	result := make(chan error, 1)
	uid, gid := currentIDs()
	args := []string{"exec", "-i", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid), c.container, "python3", "-c", workspaceServiceStreamProgram, strconv.Itoa(targetPort)}
	go func() {
		err := runner.RunWorkspaceServiceStream(ctx, args, os.Environ(), inputReader, outputWriter, io.Discard)
		_ = inputReader.CloseWithError(err)
		_ = outputWriter.CloseWithError(err)
		result <- err
	}()
	return inputWriter, outputReader, result, nil
}

func (c *workspaceServiceController) relayHTTP(ctx context.Context, exposure tobari.ServiceExposure, inbound net.Conn) {
	streamContext, cancel := context.WithCancel(ctx)
	defer cancel()
	_ = inbound.SetDeadline(time.Now().Add(workspaceServiceSetupTimeout))
	requestReader := bufio.NewReaderSize(inbound, workspaceServiceHeaderLimit)
	label, port, authorityErr := tobari.ParseServiceExposureURL(exposure.URL)
	if authorityErr != nil || port != exposure.HostPort {
		return
	}
	authority := "svc-" + label + ".localhost:" + strconv.Itoa(port)
	firstHeader, err := readServiceHeader(requestReader)
	if err != nil {
		return
	}
	firstRequest, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(firstHeader)))
	if err != nil || validateServiceRequestHeader(firstHeader, firstRequest, authority) != nil {
		return
	}
	toWorkspace, fromWorkspace, _, err := c.openWorkspaceServiceStream(streamContext, exposure.TargetPort)
	if err != nil {
		c.setExposureState(exposure.ID, tobari.ServiceStateUnavailable)
		_, _ = inbound.Write(workspaceServiceUnavailableResponse)
		return
	}
	defer toWorkspace.Close()
	defer fromWorkspace.Close()
	_ = inbound.SetDeadline(time.Time{})
	responseReader := bufio.NewReaderSize(fromWorkspace, workspaceServiceHeaderLimit)
	metas := make(chan serviceRequestMeta, 16)
	firstResponse := make(chan bool, 1)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		defer toWorkspace.Close()
		defer close(metas)
		first := true
		header := firstHeader
		request := firstRequest
		for {
			upgrade := strings.EqualFold(request.Header.Get("Upgrade"), "websocket") && strings.Contains(strings.ToLower(request.Header.Get("Connection")), "upgrade")
			result := make(chan bool, 1)
			if !upgrade {
				result = nil
			}
			select {
			case metas <- serviceRequestMeta{method: request.Method, upgrade: upgrade, upgradeResult: result}:
			case <-streamContext.Done():
				return
			}
			if _, err = toWorkspace.Write(header); err != nil {
				return
			}
			if err = copyServiceRequestBody(toWorkspace, requestReader, request); err != nil {
				return
			}
			if first {
				first = false
				select {
				case ok := <-firstResponse:
					if !ok {
						return
					}
				case <-streamContext.Done():
					return
				}
			}
			if upgrade {
				select {
				case accepted := <-result:
					if !accepted {
						continue
					}
				case <-streamContext.Done():
					return
				}
				_, _ = io.Copy(toWorkspace, requestReader)
				return
			}
			header, err = readServiceHeader(requestReader)
			if err != nil {
				return
			}
			request, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
			if err != nil || validateServiceRequestHeader(header, request, authority) != nil {
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		defer cancel()
		defer inbound.SetReadDeadline(time.Now())
		wrote := false
		for meta := range metas {
			for {
				header, err := readServiceHeader(responseReader)
				if err != nil {
					if !wrote {
						c.setExposureState(exposure.ID, tobari.ServiceStateUnavailable)
						_, _ = inbound.Write(workspaceServiceUnavailableResponse)
						select {
						case firstResponse <- false:
						default:
						}
					}
					return
				}
				response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), nil)
				if err != nil {
					return
				}
				if _, err = inbound.Write(header); err != nil {
					return
				}
				wrote = true
				c.setExposureState(exposure.ID, tobari.ServiceStateListening)
				informational := response.StatusCode >= 100 && response.StatusCode < 200 && response.StatusCode != http.StatusSwitchingProtocols
				if informational {
					continue
				}
				accepted := meta.upgrade && response.StatusCode == http.StatusSwitchingProtocols
				if meta.upgradeResult != nil {
					meta.upgradeResult <- accepted
				}
				select {
				case firstResponse <- true:
				default:
				}
				if accepted {
					_, _ = io.Copy(inbound, responseReader)
					return
				}
				keep, err := copyServiceResponseBody(inbound, responseReader, response, meta.method)
				if err != nil || !keep || response.Close {
					return
				}
				break
			}
		}
	}()
	wait.Wait()
	_ = inbound.SetDeadline(time.Time{})
}

func init() {
	// Keep the fixed body length coupled to the exact byte contract.
	if bytes.Count(workspaceServiceUnavailableResponse, []byte("\r\n\r\n")) != 1 {
		panic(fmt.Sprintf("invalid fixed service response"))
	}
}
