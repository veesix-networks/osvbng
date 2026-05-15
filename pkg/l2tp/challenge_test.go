// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"bytes"
	"crypto/md5"
	"testing"
)

func TestNewChallengeLen(t *testing.T) {
	c, err := NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != ChallengeLen {
		t.Fatalf("want %d got %d", ChallengeLen, len(c))
	}
}

func TestComputeChallengeResponseShape(t *testing.T) {
	password := []byte("test123")
	challenge := bytes.Repeat([]byte{0xAB}, ChallengeLen)
	resp := ComputeChallengeResponse(byte(MsgTypeSCCRP), password, challenge)
	if len(resp) != md5.Size {
		t.Fatalf("want %d got %d", md5.Size, len(resp))
	}
	// Recompute and compare bit-for-bit (self-consistency).
	again := ComputeChallengeResponse(byte(MsgTypeSCCRP), password, challenge)
	if !bytes.Equal(resp, again) {
		t.Fatal("deterministic compute mismatch")
	}
}

func TestComputeChallengeResponseManual(t *testing.T) {
	// Verify the construction matches the RFC's literal description:
	// MD5(messageTypeByte || password || challenge).
	pwd := []byte("secret")
	chal := []byte{0x00, 0x01, 0x02, 0x03}
	resp := ComputeChallengeResponse(0x02, pwd, chal)

	expect := md5.New()
	expect.Write([]byte{0x02})
	expect.Write(pwd)
	expect.Write(chal)
	if !bytes.Equal(resp, expect.Sum(nil)) {
		t.Fatal("hash construction does not match RFC 2661 §5.1.1")
	}
}

func TestChallengeResponseRoundTrip(t *testing.T) {
	pwd := []byte("test123")
	challenge, _ := NewChallenge()
	resp := ComputeChallengeResponse(byte(MsgTypeSCCRP), pwd, challenge)

	if err := VerifyChallengeResponse(byte(MsgTypeSCCRP), pwd, challenge, resp); err != nil {
		t.Fatalf("verify good response: %v", err)
	}

	// Wrong password.
	if err := VerifyChallengeResponse(byte(MsgTypeSCCRP), []byte("wrong"), challenge, resp); err != ErrChallengeBadResponse {
		t.Fatalf("want bad-response error, got %v", err)
	}

	// Wrong message-type byte (replay protection).
	if err := VerifyChallengeResponse(byte(MsgTypeSCCCN), pwd, challenge, resp); err != ErrChallengeBadResponse {
		t.Fatalf("want bad-response on wrong message-type, got %v", err)
	}

	// Short response.
	if err := VerifyChallengeResponse(byte(MsgTypeSCCRP), pwd, challenge, resp[:8]); err != ErrChallengeShort {
		t.Fatalf("want short error, got %v", err)
	}
}
