package snmp

import (
	"context"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// requestTimeout is how long one request may take. With one retry a dead
// device costs 10 seconds, and devices are polled at the same time.
const requestTimeout = 5 * time.Second

// snmpSession is a session over gosnmp, v2c only for now
type snmpSession struct {
	conn *gosnmp.GoSNMP
}

func dial(ctx context.Context, device Device) (session, error) {
	host, port, err := hostPort(device.Address)
	if err != nil {
		return nil, err
	}
	conn := &gosnmp.GoSNMP{
		Target:    host,
		Port:      port,
		Community: device.Community,
		Version:   gosnmp.Version2c,
		Timeout:   requestTimeout,
		Retries:   1,
		MaxOids:   maxOIDsPerGet,
		Context:   ctx,
	}
	if err := conn.Connect(); err != nil {
		return nil, err
	}
	return &snmpSession{conn: conn}, nil
}

func (s *snmpSession) Get(oids []string) (map[string]any, error) {
	dotted := make([]string, len(oids))
	for i, oid := range oids {
		dotted[i] = "." + oid
	}
	packet, err := s.conn.Get(dotted)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]any, len(packet.Variables))
	for _, pdu := range packet.Variables {
		vars[strings.TrimPrefix(pdu.Name, ".")] = value(pdu)
	}
	return vars, nil
}

func (s *snmpSession) Walk(root string, fn func(oid string, value any) error) error {
	return s.conn.BulkWalk("."+root, func(pdu gosnmp.SnmpPDU) error {
		return fn(strings.TrimPrefix(pdu.Name, "."), value(pdu))
	})
}

func (s *snmpSession) Close() error {
	if s.conn.Conn != nil {
		return s.conn.Conn.Close()
	}
	return nil
}

// value turns a PDU into the plain values the profiles read: int64 for
// INTEGER, uint64 for counters, gauges and ticks, float64 for opaque
// floats, string for octet strings and OIDs, nil when there is no such
// object
func value(pdu gosnmp.SnmpPDU) any {
	switch pdu.Type {
	case gosnmp.Integer:
		return gosnmp.ToBigInt(pdu.Value).Int64()
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Uinteger32, gosnmp.Counter64:
		return gosnmp.ToBigInt(pdu.Value).Uint64()
	case gosnmp.OpaqueFloat:
		if f, ok := pdu.Value.(float32); ok {
			return float64(f)
		}
	case gosnmp.OpaqueDouble:
		if f, ok := pdu.Value.(float64); ok {
			return f
		}
	case gosnmp.OctetString:
		if b, ok := pdu.Value.([]byte); ok {
			return string(b)
		}
	case gosnmp.ObjectIdentifier, gosnmp.IPAddress:
		if s, ok := pdu.Value.(string); ok {
			return strings.TrimPrefix(s, ".")
		}
	}
	return nil
}
