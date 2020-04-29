// Package archival contains data formats used for archival.
//
// This superseded the oonidatamodel package.
package archival

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ooni/probe-engine/model"
	"github.com/ooni/probe-engine/netx/trace"
)

// ExtSpec describes a data format extension
type ExtSpec struct {
	Name string // extension name
	V    int64  // extension version
}

// AddTo adds the current ExtSpec to the specified measurement
func (spec ExtSpec) AddTo(m *model.Measurement) {
	if m.Extensions == nil {
		m.Extensions = make(map[string]int64)
	}
	m.Extensions[spec.Name] = spec.V
}

var (
	// ExtDNS is the version of df-002-dnst.md
	ExtDNS = ExtSpec{Name: "dnst", V: 0}

	// ExtNetevents is the version of df-008-netevents.md
	ExtNetevents = ExtSpec{Name: "netevents", V: 0}

	// ExtHTTP is the version of df-001-httpt.md
	ExtHTTP = ExtSpec{Name: "httpt", V: 0}

	// ExtTCPConnect is the version of df-005-tcpconnect.md
	ExtTCPConnect = ExtSpec{Name: "tcpconnect", V: 0}

	// ExtTLSHandshake is the version of df-006-tlshandshake.md
	ExtTLSHandshake = ExtSpec{Name: "tlshandshake", V: 0}
)

// TCPConnectStatus contains the TCP connect status.
type TCPConnectStatus struct {
	Failure *string `json:"failure"`
	Success bool    `json:"success"`
}

// TCPConnectEntry contains one of the entries that are part
// of the "tcp_connect" key of a OONI report.
type TCPConnectEntry struct {
	ConnID        int64            `json:"conn_id,omitempty"`
	DialID        int64            `json:"dial_id,omitempty"`
	IP            string           `json:"ip"`
	Port          int              `json:"port"`
	Status        TCPConnectStatus `json:"status"`
	T             float64          `json:"t"`
	TransactionID int64            `json:"transaction_id,omitempty"`
}

// TCPConnectList is a list of TCPConnectEntry
type TCPConnectList []TCPConnectEntry

// NewTCPConnectList creates a new TCPConnectList
func NewTCPConnectList(begin time.Time, events []trace.Event) TCPConnectList {
	var out TCPConnectList
	for _, event := range events {
		if event.Name != "connect" {
			continue
		}
		// We assume Go is passing us legit data structures
		ip, sport, _ := net.SplitHostPort(event.Address)
		iport, _ := strconv.Atoi(sport)
		out = append(out, TCPConnectEntry{
			IP:   ip,
			Port: iport,
			Status: TCPConnectStatus{
				Failure: makeFailure(event.Err),
				Success: event.Err == nil,
			},
			T: event.Time.Sub(begin).Seconds(),
		})
	}
	return out
}

func makeFailure(err error) (s *string) {
	if err != nil {
		serio := err.Error()
		s = &serio
	}
	return
}

// HTTPTor contains Tor information
type HTTPTor struct {
	ExitIP   *string `json:"exit_ip"`
	ExitName *string `json:"exit_name"`
	IsTor    bool    `json:"is_tor"`
}

// MaybeBinaryValue is a possibly binary string. We use this helper class
// to define a custom JSON encoder that allows us to choose the proper
// representation depending on whether the Value field is valid UTF-8 or not.
type MaybeBinaryValue struct {
	Value string
}

// MarshalJSON marshals a string-like to JSON following the OONI spec that
// says that UTF-8 content is represened as string and non-UTF-8 content is
// instead represented using `{"format":"base64","data":"..."}`.
func (hb MaybeBinaryValue) MarshalJSON() ([]byte, error) {
	if utf8.ValidString(hb.Value) {
		return json.Marshal(hb.Value)
	}
	er := make(map[string]string)
	er["format"] = "base64"
	er["data"] = base64.StdEncoding.EncodeToString([]byte(hb.Value))
	return json.Marshal(er)
}

// UnmarshalJSON is the opposite of MarshalJSON.
func (hb *MaybeBinaryValue) UnmarshalJSON(d []byte) error {
	if err := json.Unmarshal(d, &hb.Value); err == nil {
		return nil
	}
	er := make(map[string]string)
	if err := json.Unmarshal(d, &er); err != nil {
		return err
	}
	if v, ok := er["format"]; !ok || v != "base64" {
		return errors.New("missing or invalid format field")
	}
	if _, ok := er["data"]; !ok {
		return errors.New("missing data field")
	}
	b64, err := base64.StdEncoding.DecodeString(er["data"])
	if err != nil {
		return err
	}
	hb.Value = string(b64)
	return nil
}

// HTTPBody is an HTTP body. As an implementation note, this type must be
// an alias for the MaybeBinaryValue type, otherwise the specific serialisation
// mechanism implemented by MaybeBinaryValue is not working.
type HTTPBody = MaybeBinaryValue

// HTTPHeaders contains HTTP headers. This headers representation is
// deprecated in favour of HTTPHeadersList since data format 0.3.0.
type HTTPHeaders map[string]MaybeBinaryValue

// HTTPHeader is a single HTTP header.
type HTTPHeader struct {
	Key   string
	Value MaybeBinaryValue
}

// MarshalJSON marshals a single HTTP header to a tuple where the first
// element is a string and the second element is maybe-binary data.
func (hh HTTPHeader) MarshalJSON() ([]byte, error) {
	if utf8.ValidString(hh.Value.Value) {
		return json.Marshal([]string{hh.Key, hh.Value.Value})
	}
	value := make(map[string]string)
	value["format"] = "base64"
	value["data"] = base64.StdEncoding.EncodeToString([]byte(hh.Value.Value))
	return json.Marshal([]interface{}{hh.Key, value})
}

// UnmarshalJSON is the opposite of MarshalJSON.
func (hh *HTTPHeader) UnmarshalJSON(d []byte) error {
	var pair []interface{}
	if err := json.Unmarshal(d, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return errors.New("unexpected pair length")
	}
	key, ok := pair[0].(string)
	if !ok {
		return errors.New("the key is not a string")
	}
	value, ok := pair[1].(string)
	if !ok {
		mapvalue, ok := pair[1].(map[string]interface{})
		if !ok {
			return errors.New("the value is neither a string nor a map[string]interface{}")
		}
		if _, ok := mapvalue["format"]; !ok {
			return errors.New("missing format")
		}
		if v, ok := mapvalue["format"].(string); !ok || v != "base64" {
			return errors.New("invalid format")
		}
		if _, ok := mapvalue["data"]; !ok {
			return errors.New("missing data field")
		}
		v, ok := mapvalue["data"].(string)
		if !ok {
			return errors.New("the data field is not a string")
		}
		b64, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return err
		}
		value = string(b64)
	}
	hh.Key, hh.Value = key, MaybeBinaryValue{Value: value}
	return nil
}

// HTTPHeadersList is a list of headers.
type HTTPHeadersList []HTTPHeader

// HTTPRequest contains an HTTP request.
//
// Headers are a map in Web Connectivity data format but
// we have added support for a list since data format version
// equal to 0.2.1 (later renamed to 0.3.0).
type HTTPRequest struct {
	Body            HTTPBody        `json:"body"`
	BodyIsTruncated bool            `json:"body_is_truncated"`
	HeadersList     HTTPHeadersList `json:"headers_list"`
	Headers         HTTPHeaders     `json:"headers"`
	Method          string          `json:"method"`
	Tor             HTTPTor         `json:"tor"`
	URL             string          `json:"url"`
}

// HTTPResponse contains an HTTP response.
//
// Headers are a map in Web Connectivity data format but
// we have added support for a list since data format version
// equal to 0.2.1 (later renamed to 0.3.0).
type HTTPResponse struct {
	Body            HTTPBody        `json:"body"`
	BodyIsTruncated bool            `json:"body_is_truncated"`
	Code            int64           `json:"code"`
	HeadersList     HTTPHeadersList `json:"headers_list"`
	Headers         HTTPHeaders     `json:"headers"`
}

// RequestEntry is one of the entries that are part of
// the "requests" key of a OONI report.
type RequestEntry struct {
	Failure       *string      `json:"failure"`
	Request       HTTPRequest  `json:"request"`
	Response      HTTPResponse `json:"response"`
	T             float64      `json:"t"`
	TransactionID int64        `json:"transaction_id,omitempty"`
}

// RequestList is a list of RequestEntry
type RequestList []RequestEntry

func addheaders(
	source http.Header,
	destList *HTTPHeadersList,
	destMap *HTTPHeaders,
) {
	for key, values := range source {
		for index, value := range values {
			value := MaybeBinaryValue{Value: value}
			// With the map representation we can only represent a single
			// value for every key. Hence the list representation.
			if index == 0 {
				(*destMap)[key] = value
			}
			*destList = append(*destList, HTTPHeader{
				Key:   key,
				Value: value,
			})
		}
	}
}

// NewRequestList returns the list for "requests"
func NewRequestList(begin time.Time, events []trace.Event) RequestList {
	var out RequestList
	// OONI's data format wants more recent request first
	for idx := len(events) - 1; idx >= 0; idx-- {
		ev := events[idx]
		if ev.Name != "http_round_trip_done" {
			continue
		}
		var entry RequestEntry
		entry.T = ev.Time.Sub(begin).Seconds()
		entry.Failure = makeFailure(ev.Err)
		if ev.HTTPRequest != nil {
			entry.Request.Headers = make(HTTPHeaders)
			addheaders(
				ev.HTTPRequest.Header, &entry.Request.HeadersList,
				&entry.Request.Headers,
			)
			entry.Request.Method = ev.HTTPRequest.Method
			entry.Request.URL = ev.HTTPRequest.URL.String()
		}
		if ev.HTTPRequestBody != nil {
			entry.Request.Body.Value = ev.HTTPRequestBody.String()
			entry.Request.BodyIsTruncated = ev.HTTPRequestBody.Truncated()
		}
		if ev.HTTPResponse != nil {
			entry.Response.Headers = make(HTTPHeaders)
			addheaders(
				ev.HTTPResponse.Header, &entry.Response.HeadersList,
				&entry.Response.Headers,
			)
			entry.Response.Code = int64(ev.HTTPResponse.StatusCode)
		}
		if ev.HTTPResponseBody != nil {
			entry.Response.Body.Value = ev.HTTPResponseBody.String()
			entry.Response.BodyIsTruncated = ev.HTTPResponseBody.Truncated()
		}
		out = append(out, entry)
	}
	return out
}

// DNSAnswerEntry is the answer to a DNS query
type DNSAnswerEntry struct {
	AnswerType string  `json:"answer_type"`
	Hostname   string  `json:"hostname,omitempty"`
	IPv4       string  `json:"ipv4,omitempty"`
	IPv6       string  `json:"ipv6,omitempty"`
	TTL        *uint32 `json:"ttl"`
}

// DNSQueryEntry is a DNS query with possibly an answer
type DNSQueryEntry struct {
	Answers          []DNSAnswerEntry `json:"answers"`
	DialID           int64            `json:"dial_id,omitempty"`
	Engine           string           `json:"engine"`
	Failure          *string          `json:"failure"`
	Hostname         string           `json:"hostname"`
	QueryType        string           `json:"query_type"`
	ResolverHostname *string          `json:"resolver_hostname"`
	ResolverPort     *string          `json:"resolver_port"`
	ResolverAddress  string           `json:"resolver_address"`
	T                float64          `json:"t"`
	TransactionID    int64            `json:"transaction_id,omitempty"`
}

type (
	// DNSQueriesList is a list of DNS queries
	DNSQueriesList []DNSQueryEntry
	dnsQueryType   string
)

// NewDNSQueriesList returns a list of DNS queries.
func NewDNSQueriesList(begin time.Time, events []trace.Event) DNSQueriesList {
	// TODO(bassosimone): add support for CNAME lookups.
	var out DNSQueriesList
	for _, ev := range events {
		if ev.Name != "resolve_done" {
			continue
		}
		for _, qtype := range []dnsQueryType{"A", "AAAA"} {
			entry := qtype.makequeryentry(begin, ev)
			for _, addr := range ev.Addresses {
				if qtype.ipoftype(addr) {
					entry.Answers = append(entry.Answers, qtype.makeanswerentry(addr))
				}
			}
			if len(entry.Answers) <= 0 {
				continue
			}
			out = append(out, entry)
		}
	}
	return out
}

func (qtype dnsQueryType) ipoftype(addr string) bool {
	switch qtype {
	case "A":
		return strings.Contains(addr, ":") == false
	case "AAAA":
		return strings.Contains(addr, ":") == true
	}
	return false
}

func (qtype dnsQueryType) makeanswerentry(addr string) DNSAnswerEntry {
	answer := DNSAnswerEntry{AnswerType: string(qtype)}
	switch qtype {
	case "A":
		answer.IPv4 = addr
	case "AAAA":
		answer.IPv6 = addr
	}
	return answer
}

func (qtype dnsQueryType) makequeryentry(begin time.Time, ev trace.Event) DNSQueryEntry {
	return DNSQueryEntry{
		Engine:          ev.Proto,
		Failure:         makeFailure(ev.Err),
		Hostname:        ev.Hostname,
		QueryType:       string(qtype),
		ResolverAddress: ev.Address,
		T:               ev.Time.Sub(begin).Seconds(),
	}
}

// NetworkEvent is a network event.
type NetworkEvent struct {
	Address       string  `json:"address,omitempty"`
	ConnID        int64   `json:"conn_id,omitempty"`
	DialID        int64   `json:"dial_id,omitempty"`
	Failure       *string `json:"failure"`
	NumBytes      int64   `json:"num_bytes,omitempty"`
	Operation     string  `json:"operation"`
	Proto         string  `json:"proto,omitempty"`
	T             float64 `json:"t"`
	TransactionID int64   `json:"transaction_id,omitempty"`
}

// NetworkEventsList is a list of network events.
type NetworkEventsList []*NetworkEvent

// NewNetworkEventsList returns a list of DNS queries.
func NewNetworkEventsList(begin time.Time, events []trace.Event) NetworkEventsList {
	var out NetworkEventsList
	for _, ev := range events {
		if ev.Name == "connect" {
			out = append(out, &NetworkEvent{
				Address:   ev.Address,
				Failure:   makeFailure(ev.Err),
				Operation: ev.Name,
				Proto:     ev.Proto,
				T:         ev.Time.Sub(begin).Seconds(),
			})
			continue
		}
		if ev.Name == "read" {
			out = append(out, &NetworkEvent{
				Failure:   makeFailure(ev.Err),
				Operation: ev.Name,
				NumBytes:  int64(ev.NumBytes),
				Proto:     ev.Proto,
				T:         ev.Time.Sub(begin).Seconds(),
			})
			continue
		}
		if ev.Name == "write" {
			out = append(out, &NetworkEvent{
				Failure:   makeFailure(ev.Err),
				Operation: ev.Name,
				NumBytes:  int64(ev.NumBytes),
				Proto:     ev.Proto,
				T:         ev.Time.Sub(begin).Seconds(),
			})
			continue
		}
		out = append(out, &NetworkEvent{
			Failure:   makeFailure(ev.Err),
			Operation: ev.Name,
			T:         ev.Time.Sub(begin).Seconds(),
		})
	}
	return out
}

// TLSHandshake contains TLS handshake data
type TLSHandshake struct {
	CipherSuite        string             `json:"cipher_suite"`
	ConnID             int64              `json:"conn_id,omitempty"`
	Failure            *string            `json:"failure"`
	NegotiatedProtocol string             `json:"negotiated_protocol"`
	PeerCertificates   []MaybeBinaryValue `json:"peer_certificates"`
	T                  float64            `json:"t"`
	TLSVersion         string             `json:"tls_version"`
	TransactionID      int64              `json:"transaction_id,omitempty"`
}

// TLSHandshakesList is a list of TLS handshakes
type TLSHandshakesList []TLSHandshake

// NewTLSHandshakesList creates a new TLSHandshakesList
func NewTLSHandshakesList(begin time.Time, events []trace.Event) TLSHandshakesList {
	var out TLSHandshakesList
	for _, ev := range events {
		if ev.Name != "tls_handshake_done" {
			continue
		}
		out = append(out, TLSHandshake{
			CipherSuite:        ev.TLSCipherSuite,
			Failure:            makeFailure(ev.Err),
			NegotiatedProtocol: ev.TLSNegotiatedProto,
			PeerCertificates:   makePeerCerts(ev.TLSPeerCerts),
			T:                  ev.Time.Sub(begin).Seconds(),
			TLSVersion:         ev.TLSVersion,
		})
	}
	return out
}

func makePeerCerts(in []*x509.Certificate) (out []MaybeBinaryValue) {
	for _, e := range in {
		out = append(out, MaybeBinaryValue{Value: string(e.Raw)})
	}
	return
}