package ppp

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

type State uint8

const (
	Initial  State = 0
	Starting State = 1
	Closed   State = 2
	Stopped  State = 3
	Closing  State = 4
	Stopping State = 5
	ReqSent  State = 6
	AckRcvd  State = 7
	AckSent  State = 8
	Opened   State = 9
)

var stateNames = []string{
	"Initial", "Starting", "Closed", "Stopped",
	"Closing", "Stopping", "Req-Sent", "Ack-Rcvd",
	"Ack-Sent", "Opened",
}

func (s State) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("State(%d)", s)
}

type Option struct {
	Type uint8
	Data []byte
}

func (o Option) Len() int {
	return 2 + len(o.Data)
}

type Callbacks struct {
	Send          func(code uint8, id uint8, data []byte)
	LayerUp       func()
	LayerDown     func()
	LayerStarted  func()
	LayerFinished func()
}

type OptionHandler interface {
	BuildConfReq() []Option
	ProcessConfReq(opts []Option) (ack, nak, rej []Option)
	ProcessConfAck(opts []Option)
	ProcessConfNak(opts []Option)
	ProcessConfRej(opts []Option)
}

type FSM struct {
	mu           sync.Mutex
	proto        uint16
	state        State
	id           uint8
	restartCount int
	failCount    int
	maxConf      int
	maxTerm      int
	maxFail      int
	restartTime  time.Duration
	timer        *time.Timer
	callbacks    Callbacks
	handler      OptionHandler
	lastReqID    uint8
}

func NewFSM(proto uint16, cb Callbacks, handler OptionHandler) *FSM {
	return &FSM{
		proto:       proto,
		state:       Initial,
		maxConf:     10,
		maxTerm:     2,
		maxFail:     5,
		restartTime: 3 * time.Second,
		callbacks:   cb,
		handler:     handler,
	}
}

func (f *FSM) State() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *FSM) Up() {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch f.state {
	case Initial:
		f.state = Closed
	case Starting:
		f.irc()
		f.scr()
		f.state = ReqSent
	}
}

func (f *FSM) Down() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stopTimer()
	switch f.state {
	case Closed, Closing:
		f.state = Initial
	case Stopped:
		f.tls()
		f.state = Starting
	case Stopping, ReqSent, AckRcvd, AckSent:
		f.state = Starting
	case Opened:
		f.tld()
		f.state = Starting
	}
}

func (f *FSM) Open() {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch f.state {
	case Initial:
		f.tls()
		f.state = Starting
	case Closed:
		f.irc()
		f.scr()
		f.state = ReqSent
	}
}

func (f *FSM) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch f.state {
	case Starting:
		f.tlf()
		f.state = Initial
	case Stopped:
		f.state = Closed
	case Stopping:
		f.state = Closing
	case Opened:
		f.tld()
		f.irc()
		f.str()
		f.state = Closing
	case ReqSent, AckRcvd, AckSent:
		f.irc()
		f.str()
		f.state = Closing
	}
}

func (f *FSM) Input(code uint8, id uint8, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch code {
	case ConfReq:
		f.rcrEvent(id, data)
	case ConfAck:
		f.rcaEvent(id, data)
	case ConfNak, ConfRej:
		f.rcnEvent(id, data, code == ConfRej)
	case TermReq:
		f.rtrEvent(id)
	case TermAck:
		f.rtaEvent()
	case CodeRej:
		f.rxjEvent(data)
	case EchoReq:
		f.rxrEvent(id, data)
	case EchoRep, DiscReq:
		// silently discard
	default:
		f.rucEvent(code, id, data)
	}
}

func (f *FSM) Timeout() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.restartCount > 0 {
		f.restartCount--
		switch f.state {
		case Closing, Stopping:
			f.str()
		case ReqSent, AckSent:
			f.scr()
		case AckRcvd:
			f.scr()
			f.state = ReqSent
		}
	} else {
		switch f.state {
		case Closing:
			f.tlf()
			f.state = Closed
		case Stopping:
			f.tlf()
			f.state = Stopped
		case ReqSent, AckRcvd, AckSent:
			f.tlf()
			f.state = Stopped
		}
	}
}

func (f *FSM) rcrEvent(id uint8, data []byte) {
	opts, err := ParseOptions(data)
	if err != nil {
		return
	}

	ack, nak, rej := f.handler.ProcessConfReq(opts)
	isGood := len(nak) == 0 && len(rej) == 0

	switch f.state {
	case Closed:
		f.sta(id)
	case Stopped:
		f.irc()
		f.scr()
		if isGood {
			f.sca(id, ack)
			f.state = AckSent
		} else if len(rej) > 0 {
			f.scj(id, rej)
			f.state = ReqSent
		} else {
			f.scn(id, nak)
			f.state = ReqSent
		}
	case ReqSent:
		if isGood {
			f.sca(id, ack)
			f.state = AckSent
		} else if len(rej) > 0 {
			f.scj(id, rej)
		} else {
			f.scn(id, nak)
		}
	case AckRcvd:
		if isGood {
			f.sca(id, ack)
			f.stopTimer()
			f.tlu()
			f.state = Opened
		} else if len(rej) > 0 {
			f.scj(id, rej)
		} else {
			f.scn(id, nak)
		}
	case AckSent:
		if isGood {
			f.sca(id, ack)
		} else if len(rej) > 0 {
			f.scj(id, rej)
			f.state = ReqSent
		} else {
			f.scn(id, nak)
			f.state = ReqSent
		}
	case Opened:
		f.tld()
		f.scr()
		if isGood {
			f.sca(id, ack)
			f.state = AckSent
		} else if len(rej) > 0 {
			f.scj(id, rej)
			f.state = ReqSent
		} else {
			f.scn(id, nak)
			f.state = ReqSent
		}
	}
}

func (f *FSM) rcaEvent(id uint8, data []byte) {
	if id != f.lastReqID {
		return
	}

	opts, _ := ParseOptions(data)
	f.handler.ProcessConfAck(opts)

	switch f.state {
	case Closed, Stopped:
		f.sta(id)
	case ReqSent:
		f.irc()
		f.state = AckRcvd
	case AckRcvd:
		f.scr()
		f.state = ReqSent
	case AckSent:
		f.stopTimer()
		f.tlu()
		f.state = Opened
	case Opened:
		f.tld()
		f.scr()
		f.state = ReqSent
	}
}

func (f *FSM) rcnEvent(id uint8, data []byte, isRej bool) {
	if id != f.lastReqID {
		return
	}

	opts, _ := ParseOptions(data)
	if isRej {
		f.handler.ProcessConfRej(opts)
	} else {
		f.handler.ProcessConfNak(opts)
	}

	switch f.state {
	case Closed, Stopped:
		f.sta(id)
	case ReqSent:
		f.irc()
		f.scr()
	case AckRcvd:
		f.scr()
		f.state = ReqSent
	case AckSent:
		f.irc()
		f.scr()
		f.state = ReqSent
	case Opened:
		f.tld()
		f.scr()
		f.state = ReqSent
	}
}

func (f *FSM) rtrEvent(id uint8) {
	switch f.state {
	case Closed, Stopped, Closing, Stopping:
		f.sta(id)
	case ReqSent, AckRcvd, AckSent:
		f.sta(id)
	case Opened:
		f.tld()
		f.zrc()
		f.sta(id)
		f.state = Stopping
	}
}

func (f *FSM) rtaEvent() {
	switch f.state {
	case Closing:
		f.stopTimer()
		f.tlf()
		f.state = Closed
	case Stopping:
		f.stopTimer()
		f.tlf()
		f.state = Stopped
	case AckSent:
		f.state = ReqSent
	case Opened:
		f.tld()
		f.scr()
		f.state = ReqSent
	}
}

func (f *FSM) rxjEvent(data []byte) {
	switch f.state {
	case ReqSent, AckRcvd, AckSent:
		f.tlf()
		f.state = Stopped
	case Opened:
		f.tld()
		f.irc()
		f.str()
		f.state = Stopping
	}
}

func (f *FSM) rucEvent(code uint8, id uint8, data []byte) {
	pkt := make([]byte, 4+len(data))
	pkt[0] = code
	pkt[1] = id
	binary.BigEndian.PutUint16(pkt[2:4], uint16(4+len(data)))
	copy(pkt[4:], data)
	f.send(CodeRej, f.nextID(), pkt)
}

func (f *FSM) rxrEvent(id uint8, data []byte) {
	if f.state == Opened && len(data) >= 4 {
		f.send(EchoRep, id, data)
	}
}

func (f *FSM) irc() {
	f.restartCount = f.maxConf
}

func (f *FSM) zrc() {
	f.restartCount = 0
}

func (f *FSM) scr() {
	opts := f.handler.BuildConfReq()
	data := SerializeOptions(opts)
	id := f.nextID()
	f.lastReqID = id
	f.send(ConfReq, id, data)
	f.startTimer()
}

func (f *FSM) sca(id uint8, opts []Option) {
	f.send(ConfAck, id, SerializeOptions(opts))
}

func (f *FSM) scn(id uint8, opts []Option) {
	f.failCount++
	f.send(ConfNak, id, SerializeOptions(opts))
}

func (f *FSM) scj(id uint8, opts []Option) {
	f.send(ConfRej, id, SerializeOptions(opts))
}

func (f *FSM) str() {
	f.restartCount = f.maxTerm
	f.send(TermReq, f.nextID(), nil)
	f.startTimer()
}

func (f *FSM) sta(id uint8) {
	f.send(TermAck, id, nil)
}

func (f *FSM) tlu() {
	if f.callbacks.LayerUp != nil {
		f.callbacks.LayerUp()
	}
}

func (f *FSM) tld() {
	if f.callbacks.LayerDown != nil {
		f.callbacks.LayerDown()
	}
}

func (f *FSM) tls() {
	if f.callbacks.LayerStarted != nil {
		f.callbacks.LayerStarted()
	}
}

func (f *FSM) tlf() {
	if f.callbacks.LayerFinished != nil {
		f.callbacks.LayerFinished()
	}
}

func (f *FSM) send(code uint8, id uint8, data []byte) {
	if f.callbacks.Send != nil {
		f.callbacks.Send(code, id, data)
	}
}

func (f *FSM) nextID() uint8 {
	f.id++
	return f.id
}

func (f *FSM) startTimer() {
	f.stopTimer()
	f.timer = time.AfterFunc(f.restartTime, f.Timeout)
}

func (f *FSM) stopTimer() {
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
}

func (f *FSM) Kill() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stopTimer()
	f.state = Closed
}

func ParseOptions(data []byte) ([]Option, error) {
	var opts []Option
	for len(data) >= 2 {
		t := data[0]
		l := int(data[1])
		if l < 2 || l > len(data) {
			return nil, fmt.Errorf("invalid option length")
		}
		opts = append(opts, Option{Type: t, Data: append([]byte(nil), data[2:l]...)})
		data = data[l:]
	}
	return opts, nil
}

func SerializeOptions(opts []Option) []byte {
	var buf []byte
	for _, o := range opts {
		buf = append(buf, o.Type, uint8(2+len(o.Data)))
		buf = append(buf, o.Data...)
	}
	return buf
}
