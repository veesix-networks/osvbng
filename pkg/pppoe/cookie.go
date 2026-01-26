package pppoe

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"net"
	"time"
)

const (
	cookieHMACSize   = 32
	cookieTotalSize  = 36
	cookieDefaultTTL = 60 * time.Second
)

type CookieManager struct {
	secret []byte
	ttl    time.Duration
}

func NewCookieManager(ttl time.Duration) (*CookieManager, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}

	if ttl == 0 {
		ttl = cookieDefaultTTL
	}

	return &CookieManager{
		secret: secret,
		ttl:    ttl,
	}, nil
}

func (cm *CookieManager) Generate(mac net.HardwareAddr, svlan, cvlan uint16) []byte {
	timestamp := uint32(time.Now().Unix())

	tsBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(tsBytes, timestamp)

	data := make([]byte, 0, 14)
	data = append(data, mac...)
	data = binary.BigEndian.AppendUint16(data, svlan)
	data = binary.BigEndian.AppendUint16(data, cvlan)
	data = append(data, tsBytes...)

	h := hmac.New(sha256.New, cm.secret)
	h.Write(data)
	sig := h.Sum(nil)

	cookie := make([]byte, cookieTotalSize)
	copy(cookie[:cookieHMACSize], sig)
	copy(cookie[cookieHMACSize:], tsBytes)

	return cookie
}

func (cm *CookieManager) Validate(cookie []byte, mac net.HardwareAddr, svlan, cvlan uint16) bool {
	if len(cookie) != cookieTotalSize {
		return false
	}

	ts := binary.BigEndian.Uint32(cookie[cookieHMACSize:])
	cookieTime := time.Unix(int64(ts), 0)

	if time.Since(cookieTime) > cm.ttl {
		return false
	}

	data := make([]byte, 0, 10)
	data = append(data, mac...)
	data = append(data, byte(svlan>>8), byte(svlan))
	data = append(data, byte(cvlan>>8), byte(cvlan))
	data = append(data, cookie[cookieHMACSize:]...)

	h := hmac.New(sha256.New, cm.secret)
	h.Write(data)
	expected := h.Sum(nil)

	return hmac.Equal(cookie[:cookieHMACSize], expected)
}
