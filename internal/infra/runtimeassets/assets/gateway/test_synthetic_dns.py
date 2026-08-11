import ipaddress
import struct
import unittest

import synthetic_dns


def query(name="example.com", query_type=1, query_class=1, edns=False):
    labels = b"".join(
        bytes([len(label)]) + label.encode("ascii") for label in name.split(".")
    )
    message = (
        struct.pack("!HHHHHH", 0x1234, 0x0100, 1, 0, 0, int(edns))
        + labels
        + b"\x00"
        + struct.pack("!HH", query_type, query_class)
    )
    if edns:
        message += b"\x00" + struct.pack("!HHIH", 41, 4096, 0, 0)
    return message


class SyntheticDNSTests(unittest.TestCase):
    def test_a_query_gets_one_zero_ttl_non_public_answer(self):
        response = synthetic_dns.build_response(query())
        header = struct.unpack("!HHHHHH", response[:12])
        self.assertEqual(header, (0x1234, 0x8500, 1, 1, 0, 0))
        self.assertIn(synthetic_dns.SYNTHETIC_IPV4.packed, response)
        self.assertFalse(ipaddress.ip_address(synthetic_dns.SYNTHETIC_IPV4).is_global)

    def test_unsupported_record_is_noerror_nodata(self):
        response = synthetic_dns.build_response(query(query_type=28))
        self.assertEqual(struct.unpack("!HHHHHH", response[:12]), (0x1234, 0x8500, 1, 0, 0, 0))
        self.assertNotIn(synthetic_dns.SYNTHETIC_IPV4.packed, response)

    def test_unsupported_class_is_refused(self):
        response = synthetic_dns.build_response(query(query_class=3))
        flags = struct.unpack("!H", response[2:4])[0]
        self.assertEqual(flags & 0xF, 5)

    def test_docker_edns_query_gets_the_same_bounded_answer(self):
        response = synthetic_dns.build_response(query(edns=True))
        self.assertEqual(struct.unpack("!HHHHHH", response[:12]), (0x1234, 0x8500, 1, 1, 0, 0))
        self.assertIn(synthetic_dns.SYNTHETIC_IPV4.packed, response)

    def test_non_opt_or_malformed_additional_record_is_rejected(self):
        non_opt = bytearray(query(edns=True))
        non_opt[-10:-8] = struct.pack("!H", 16)
        malformed = query(edns=True) + b"x"
        for message in (bytes(non_opt), malformed):
            with self.subTest(length=len(message)):
                with self.assertRaises(synthetic_dns.DNSMessageError):
                    synthetic_dns.build_response(message)

    def test_compression_multiple_questions_and_oversize_are_rejected(self):
        compressed = query()
        compressed = compressed[:12] + b"\xc0\x0c" + compressed[-4:]
        multiple = bytearray(query())
        multiple[4:6] = struct.pack("!H", 2)
        for message in (compressed, bytes(multiple), b"x" * (synthetic_dns.MAX_TCP_REQUEST + 1)):
            with self.subTest(length=len(message)):
                with self.assertRaises(synthetic_dns.DNSMessageError):
                    synthetic_dns.build_response(message)

    def test_source_limiter_has_fixed_window_and_capacity(self):
        limiter = synthetic_dns.SourceLimiter()
        for _ in range(synthetic_dns.RATE_REQUESTS_PER_WINDOW):
            self.assertTrue(limiter.allow("192.0.2.1", now=1.0))
        self.assertFalse(limiter.allow("192.0.2.1", now=1.0))
        self.assertTrue(limiter.allow("192.0.2.1", now=2.1))
        full = synthetic_dns.SourceLimiter()
        for index in range(synthetic_dns.MAX_SOURCES):
            self.assertTrue(full.allow(f"source-{index}", now=1.0))
        self.assertFalse(full.allow("overflow", now=1.0))


if __name__ == "__main__":
    unittest.main()
