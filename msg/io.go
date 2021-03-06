package msg

import (
	"MultiEx/util"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// ReadMsg read bytes from reader and convert them to a message.
func ReadMsg(r io.Reader) (m Message, e error, retry bool) {
	// Read size of message.
	var size int16
	e = binary.Read(r, binary.LittleEndian, &size)
	if e != nil {
		return
	}
	// Read message bytes and convert to json object.
	bytes := make([]byte, size)
	rSize, e := r.Read(bytes)
	if e != nil {
		return
	}
	if int16(rSize) != size {
		e = fmt.Errorf("read size is not equal original size")
		return
	}
	var pkg pack
	e = json.Unmarshal(bytes, &pkg)
	if e != nil {
		retry = true
		return
	}
	switch pkg.Typ {
	case "NewClient":
		m = &NewClient{}
	case "ReNewClient":
		m = &ReNewClient{}
	case "NewProxy":
		m = &NewProxy{}
	case "CloseProxy":
		m = &CloseProxy{}
	//case "ActivateProxy":
	//	m = &ActivateProxy{}
	case "ForwardInfo":
		m = &ForwardInfo{}
	case "Ping":
		m = &Ping{}
	case "Pong":
		m = &Pong{}
	case "PortInUse":
		m = &PortInUse{}
	case "CloseCtrl":
		m = &CloseCtrl{}
	case "GResponse":
		m = &GResponse{}
	case "ClientNotExist":
		m = &ClientNotExist{}
	default:
		e = fmt.Errorf("cannot parse connection type")
		return
	}
	e = json.Unmarshal(pkg.Msg, m)
	return
}

// WriteMsg write message to writer.
func WriteMsg(w io.Writer, msg Message) (e error) {

	typ := reflect.TypeOf(msg).Name()

	pBytes, e := json.Marshal(struct {
		Typ string
		Msg interface{}
	}{
		Typ: typ,
		Msg: msg,
	})
	if e != nil {
		return
	}

	buffer := new(bytes.Buffer)
	e = binary.Write(buffer, binary.LittleEndian, int16(len(pBytes)))
	if e != nil {
		return
	}
	composite := util.BytesCombine(buffer.Bytes(), pBytes)
	l, e := w.Write(composite)
	if e != nil {
		return
	}
	if l != len(composite) {
		e = fmt.Errorf("write package to writer failed")
	}
	return
}
