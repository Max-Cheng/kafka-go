package protocol

import (
	"io"
)

func ReadRequest(r io.Reader) (apiVersion int16, correlationID int32, clientID string, msg Message, err error) {
	d := &decoder{reader: r, remain: 4}
	size := d.readInt32()

	if err = d.err; err != nil {
		return
	}

	d.remain = int(size)
	defer d.discardAll()

	apiKey := d.readInt16()
	apiVersion = d.readInt16()
	correlationID = d.readInt32()
	clientID = d.readString()

	if i := int(apiKey); i < 0 || i >= len(apiTypes) {
		err = errorf("unsupported api key: %d", i)
		return
	}

	if d.err != nil {
		err = d.err
		return
	}

	t := &apiTypes[apiKey]
	if t == nil {
		err = errorf("unsupported api: %s", apiNames[apiKey])
		return
	}

	minVersion := t.minVersion()
	maxVersion := t.maxVersion()

	if apiVersion < minVersion || apiVersion > maxVersion {
		err = errorf("unsupported %s version: v%d not in range v%d-v%d",
			ApiKey(apiKey), apiVersion, minVersion, maxVersion)
		return
	}

	req := &t.requests[apiVersion-minVersion]
	msg = req.new()
	req.decode(d, valueOf(msg))
	err = d.err
	return
}

func WriteRequest(w io.Writer, apiVersion int16, correlationID int32, clientID string, msg Message) error {
	apiKey := msg.ApiKey()

	if i := int(apiKey); i < 0 || i >= len(apiTypes) {
		return errorf("unsupported api key: %d", i)
	}

	t := &apiTypes[apiKey]
	if t == nil {
		return errorf("unsupported api: %s", apiNames[apiKey])
	}

	minVersion := t.minVersion()
	maxVersion := t.maxVersion()

	if apiVersion < minVersion || apiVersion > maxVersion {
		return errorf("unsupported %s version: v%d not in range v%d-v%d",
			ApiKey(apiKey), apiVersion, minVersion, maxVersion)
	}

	r := &t.requests[apiVersion-minVersion]
	v := valueOf(msg)
	b := newPageBuffer()
	defer b.unref()

	e := &encoder{writer: b}
	e.writeInt32(0) // placeholder for the request size
	e.writeInt16(int16(apiKey))
	e.writeInt16(apiVersion)
	e.writeInt32(correlationID)
	// Technically, recent versions of kafka interpret this field as a nullable
	// string, however kafka 0.10 expected a non-nullable string and fails with
	// a NullPointerExecption when sent a null client id.
	e.writeString(clientID)
	r.encode(e, v)

	size := packUint32(uint32(b.Size()) - 4)
	b.WriteAt(size[:], 0)

	_, err := b.WriteTo(w)
	return err
}
