#!/bin/sh
set -eu

revision=v1
table=tobari_guard_v1
transparent_port=15001
dns_port=15053

fail() {
  echo "network guard: $1" >&2
  exit 1
}

[ "$(id -u)" -eq 0 ] || fail "root is required"
command -v ip >/dev/null 2>&1 || fail "iproute2 is unavailable"
command -v nft >/dev/null 2>&1 || fail "nftables is unavailable"

install_gateway() {
  [ "$(cat /proc/sys/net/ipv4/ip_forward)" = 0 ] || fail "IPv4 forwarding is enabled"
  [ "$(cat /proc/sys/net/ipv6/conf/all/forwarding)" = 0 ] || fail "IPv6 forwarding is enabled"
  replacement=
  if nft list table inet "$table" >/dev/null 2>&1; then
    replacement="delete table inet $table"
  fi
  nft -f - <<EOF
$replacement
table inet $table {
  chain prerouting {
    type nat hook prerouting priority dstnat; policy accept;
    meta l4proto udp udp dport 53 redirect to :$dns_port
    meta l4proto tcp tcp dport 53 redirect to :$dns_port
    fib daddr type local tcp dport { $transparent_port, $dns_port } return
    meta l4proto tcp redirect to :$transparent_port
  }
  chain forward {
    type filter hook forward priority filter; policy drop;
  }
}
EOF
  [ "$(cat /proc/sys/net/ipv4/ip_forward)" = 0 ] || fail "IPv4 forwarding changed"
  [ "$(cat /proc/sys/net/ipv6/conf/all/forwarding)" = 0 ] || fail "IPv6 forwarding changed"
  nft list chain inet "$table" forward | grep -Fq 'policy drop' || fail "forward policy is not closed"
  nft list chain inet "$table" prerouting | grep -Fq "redirect to :$transparent_port" || fail "transparent redirect is missing"
  nft list chain inet "$table" prerouting | grep -Fq "redirect to :$dns_port" || fail "DNS redirect is missing"
  printf 'tobari-network-guard %s gateway\n' "$revision"
}

validate_workspace_input() {
  [ "$#" -eq 2 ] || fail "Workspace guard requires Gateway address and subnet"
  validated=$(python3 - "$1" "$2" <<'PY'
import ipaddress
import sys

try:
    gateway = ipaddress.IPv4Address(sys.argv[1])
    subnet = ipaddress.IPv4Network(sys.argv[2], strict=True)
except ValueError:
    raise SystemExit(1)
if (
    gateway.is_unspecified
    or gateway.is_loopback
    or gateway.is_link_local
    or gateway.is_multicast
    or gateway not in subnet
):
    raise SystemExit(1)
print(gateway)
print(subnet)
PY
  ) || fail "Workspace network input is invalid"
  gateway_ip=$(printf '%s\n' "$validated" | sed -n '1p')
  subnet=$(printf '%s\n' "$validated" | sed -n '2p')
  [ "$gateway_ip" = "$1" ] || fail "Gateway address is not canonical"
  [ "$subnet" = "$2" ] || fail "subnet is not canonical"
}

install_workspace() {
  validate_workspace_input "$@"
  interface=$(ip -4 route show "$subnet" | awk 'NR == 1 && $2 == "dev" { print $3 }')
  [ -n "$interface" ] || fail "Workspace interface is unavailable"
  case $interface in
    *[!A-Za-z0-9_.-]*|'') fail "Workspace interface is invalid" ;;
  esac
  replacement=
  if nft list table inet "$table" >/dev/null 2>&1; then
    replacement="delete table inet $table"
  fi
  nft -f - <<EOF
$replacement
table inet $table {
  chain output {
    type filter hook output priority filter; policy drop;
    oifname "lo" accept
    ct state established,related accept
    ip daddr $gateway_ip meta l4proto udp udp dport 53 accept
    ip daddr $gateway_ip meta l4proto tcp tcp dport 53 accept
    meta nfproto ipv6 reject with icmpv6 type admin-prohibited
    ip daddr $subnet reject with icmp type admin-prohibited
    meta l4proto tcp accept
    meta l4proto udp udp dport 53 accept
    meta l4proto udp reject with icmp type port-unreachable
  }
}
EOF
  ip -4 route replace default via "$gateway_ip" dev "$interface"
  default_route=$(ip -4 route show default | awk '
    NF { count += 1; route = $1 " " $2 " " $3 " " $4 " " $5 }
    END { if (count == 1) print route }
  ')
  [ "$default_route" = "default via $gateway_ip dev $interface" ] || fail "Workspace default route is invalid"
  nft list chain inet "$table" output | grep -Fq 'policy drop' || fail "Workspace output policy is not closed"
  nft list chain inet "$table" output | grep -Fq 'reject with icmpv6 admin-prohibited' || fail "Workspace IPv6 rejection is missing"
  nft list chain inet "$table" output | grep -Fq "ip daddr $subnet reject" || fail "Workspace peer rejection is missing"
  printf 'tobari-network-guard %s workspace\n' "$revision"
}

case ${1:-} in
  gateway)
    [ "$#" -eq 1 ] || fail "Gateway guard accepts no arguments"
    install_gateway
    ;;
  workspace)
    shift
    install_workspace "$@"
    ;;
  *) fail "mode must be gateway or workspace" ;;
esac
