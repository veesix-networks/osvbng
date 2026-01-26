package ppp

import (
	"crypto/md5"
	"encoding/binary"
)

type PAPHandler struct {
	Send     func(code uint8, id uint8, data []byte)
	Validate func(username, password string) bool
	OnResult func(success bool, message string)
}

func (p *PAPHandler) SendAuthReq(id uint8, username, password string) {
	data := make([]byte, 2+len(username)+len(password))
	data[0] = uint8(len(username))
	copy(data[1:1+len(username)], username)
	data[1+len(username)] = uint8(len(password))
	copy(data[2+len(username):], password)
	p.Send(PAPAuthReq, id, data)
}

func (p *PAPHandler) HandleAuthReq(id uint8, data []byte) {
	if len(data) < 2 {
		p.sendAuthNak(id, "malformed request")
		return
	}

	userLen := int(data[0])
	if len(data) < 1+userLen+1 {
		p.sendAuthNak(id, "malformed request")
		return
	}
	username := string(data[1 : 1+userLen])

	passLen := int(data[1+userLen])
	if len(data) < 2+userLen+passLen {
		p.sendAuthNak(id, "malformed request")
		return
	}
	password := string(data[2+userLen : 2+userLen+passLen])

	if p.Validate != nil && p.Validate(username, password) {
		p.sendAuthAck(id, "authentication successful")
	} else {
		p.sendAuthNak(id, "authentication failed")
	}
}

func (p *PAPHandler) HandleAuthAck(id uint8, data []byte) {
	msg := ""
	if len(data) > 0 {
		msgLen := int(data[0])
		if len(data) > msgLen {
			msg = string(data[1 : 1+msgLen])
		}
	}
	if p.OnResult != nil {
		p.OnResult(true, msg)
	}
}

func (p *PAPHandler) HandleAuthNak(id uint8, data []byte) {
	msg := ""
	if len(data) > 0 {
		msgLen := int(data[0])
		if len(data) > msgLen {
			msg = string(data[1 : 1+msgLen])
		}
	}
	if p.OnResult != nil {
		p.OnResult(false, msg)
	}
}

func (p *PAPHandler) sendAuthAck(id uint8, msg string) {
	data := make([]byte, 1+len(msg))
	data[0] = uint8(len(msg))
	copy(data[1:], msg)
	p.Send(PAPAuthAck, id, data)
}

func (p *PAPHandler) sendAuthNak(id uint8, msg string) {
	data := make([]byte, 1+len(msg))
	data[0] = uint8(len(msg))
	copy(data[1:], msg)
	p.Send(PAPAuthNak, id, data)
}

type CHAPHandler struct {
	Send       func(code uint8, id uint8, data []byte)
	GetSecret  func(username string) string
	OnResult   func(success bool, message string)
	LocalName  string
	LocalSecret string
}

func (c *CHAPHandler) SendChallenge(id uint8, challenge []byte, name string) {
	data := make([]byte, 1+len(challenge)+len(name))
	data[0] = uint8(len(challenge))
	copy(data[1:1+len(challenge)], challenge)
	copy(data[1+len(challenge):], name)
	c.Send(CHAPChallenge, id, data)
}

func (c *CHAPHandler) HandleChallenge(id uint8, data []byte) {
	if len(data) < 1 {
		return
	}

	valueLen := int(data[0])
	if len(data) < 1+valueLen {
		return
	}
	challenge := data[1 : 1+valueLen]
	_ = string(data[1+valueLen:])

	response := c.computeMD5Response(id, challenge, c.LocalSecret)
	respData := make([]byte, 1+len(response)+len(c.LocalName))
	respData[0] = uint8(len(response))
	copy(respData[1:1+len(response)], response)
	copy(respData[1+len(response):], c.LocalName)
	c.Send(CHAPResponse, id, respData)
}

func (c *CHAPHandler) HandleResponse(id uint8, data []byte, challenge []byte) {
	if len(data) < 1 {
		c.sendFailure(id, "malformed response")
		return
	}

	valueLen := int(data[0])
	if len(data) < 1+valueLen {
		c.sendFailure(id, "malformed response")
		return
	}
	response := data[1 : 1+valueLen]
	name := string(data[1+valueLen:])

	secret := ""
	if c.GetSecret != nil {
		secret = c.GetSecret(name)
	}

	expected := c.computeMD5Response(id, challenge, secret)
	if len(response) == len(expected) && constantTimeCompare(response, expected) {
		c.sendSuccess(id, "authentication successful")
		if c.OnResult != nil {
			c.OnResult(true, "authentication successful")
		}
	} else {
		c.sendFailure(id, "authentication failed")
		if c.OnResult != nil {
			c.OnResult(false, "authentication failed")
		}
	}
}

func (c *CHAPHandler) HandleSuccess(id uint8, data []byte) {
	if c.OnResult != nil {
		c.OnResult(true, string(data))
	}
}

func (c *CHAPHandler) HandleFailure(id uint8, data []byte) {
	if c.OnResult != nil {
		c.OnResult(false, string(data))
	}
}

func (c *CHAPHandler) sendSuccess(id uint8, msg string) {
	c.Send(CHAPSuccess, id, []byte(msg))
}

func (c *CHAPHandler) sendFailure(id uint8, msg string) {
	c.Send(CHAPFailure, id, []byte(msg))
}

func (c *CHAPHandler) computeMD5Response(id uint8, challenge []byte, secret string) []byte {
	h := md5.New()
	h.Write([]byte{id})
	h.Write([]byte(secret))
	h.Write(challenge)
	return h.Sum(nil)
}

func constantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

func ParsePAPPacket(data []byte) (code uint8, id uint8, payload []byte, err error) {
	if len(data) < 4 {
		return 0, 0, nil, ErrShortPacket
	}
	code = data[0]
	id = data[1]
	length := binary.BigEndian.Uint16(data[2:4])
	if int(length) > len(data) {
		return 0, 0, nil, ErrShortPacket
	}
	return code, id, data[4:length], nil
}

func ParseCHAPPacket(data []byte) (code uint8, id uint8, payload []byte, err error) {
	if len(data) < 4 {
		return 0, 0, nil, ErrShortPacket
	}
	code = data[0]
	id = data[1]
	length := binary.BigEndian.Uint16(data[2:4])
	if int(length) > len(data) {
		return 0, 0, nil, ErrShortPacket
	}
	return code, id, data[4:length], nil
}

type ErrType int

const (
	ErrShortPacket errString = "short packet"
)

type errString string

func (e errString) Error() string { return string(e) }
