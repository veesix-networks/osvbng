# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

# Packet capture for rig assertions, run inside a lab container that has
# python3 but no tcpdump:
#
#   docker exec -i <container> python3 - MODE IFACE TIMEOUT ARG < capture.py
#
# Prints one line of key=value pairs for the first matching frame and exits
# 0, or prints "timeout" and exits 1.
#
#   icmp-error IFACE TIMEOUT SRC_IP    first ICMP error (destination
#                                      unreachable or time exceeded) whose
#                                      outer source is SRC_IP, with the
#                                      quoted inner tuple
#   tcp-syn    IFACE TIMEOUT DST_PORT  first TCP SYN (ACK clear) to DST_PORT,
#                                      with its MSS option

import socket
import struct
import sys
import time


def ipv4(b):
    return "%d.%d.%d.%d" % tuple(b)


def parse_ip(data):
    if len(data) < 20:
        return None
    ihl = (data[0] & 0x0F) * 4
    if data[0] >> 4 != 4 or len(data) < ihl:
        return None
    return {
        "proto": data[9],
        "src": ipv4(data[12:16]),
        "dst": ipv4(data[16:20]),
        "l4": data[ihl:],
    }


def parse_frame(frame):
    off = 12
    ethertype = struct.unpack("!H", frame[off:off + 2])[0]
    while ethertype in (0x8100, 0x88A8):
        off += 4
        ethertype = struct.unpack("!H", frame[off:off + 2])[0]
    if ethertype != 0x0800:
        return None
    return parse_ip(frame[off + 2:])


def match_icmp_error(pkt, src_ip):
    if pkt["proto"] != 1 or pkt["src"] != src_ip or len(pkt["l4"]) < 8:
        return None
    icmp_type, code = pkt["l4"][0], pkt["l4"][1]
    if icmp_type not in (3, 11):
        return None
    inner = parse_ip(pkt["l4"][8:])
    if inner is None or len(inner["l4"]) < 8:
        return None
    if inner["proto"] in (6, 17):
        sport, dport = struct.unpack("!HH", inner["l4"][:4])
    elif inner["proto"] == 1:
        sport = dport = struct.unpack("!H", inner["l4"][4:6])[0]
    else:
        sport = dport = 0
    return ("type=%d code=%d inner_proto=%d inner_src=%s inner_sport=%d "
            "inner_dst=%s inner_dport=%d" % (icmp_type, code, inner["proto"],
                                             inner["src"], sport,
                                             inner["dst"], dport))


def match_tcp_syn(pkt, dst_port):
    if pkt["proto"] != 6 or len(pkt["l4"]) < 20:
        return None
    tcp = pkt["l4"]
    sport, dport = struct.unpack("!HH", tcp[:4])
    flags = tcp[13]
    if dport != dst_port or not (flags & 0x02) or (flags & 0x10):
        return None
    doff = (tcp[12] >> 4) * 4
    opts = tcp[20:doff]
    mss = 0
    i = 0
    while i < len(opts):
        kind = opts[i]
        if kind == 0:
            break
        if kind == 1:
            i += 1
            continue
        if i + 1 >= len(opts):
            break
        length = opts[i + 1]
        if kind == 2 and length == 4:
            mss = struct.unpack("!H", opts[i + 2:i + 4])[0]
        i += max(length, 2)
    return "src=%s sport=%d dst=%s dport=%d mss=%d" % (
        pkt["src"], sport, pkt["dst"], dport, mss)


def main():
    mode, iface, timeout, arg = sys.argv[1], sys.argv[2], float(sys.argv[3]), sys.argv[4]
    sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x0003))
    sock.bind((iface, 0))
    sock.settimeout(0.5)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            frame = sock.recv(65535)
        except socket.timeout:
            continue
        pkt = parse_frame(frame)
        if pkt is None:
            continue
        if mode == "icmp-error":
            line = match_icmp_error(pkt, arg)
        elif mode == "tcp-syn":
            line = match_tcp_syn(pkt, int(arg))
        else:
            sys.exit("unknown mode %s" % mode)
        if line:
            print(line, flush=True)
            return 0
    print("timeout", flush=True)
    return 1


if __name__ == "__main__":
    sys.exit(main())
