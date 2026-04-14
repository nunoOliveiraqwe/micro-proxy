package abuseipdb

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const blacklistEndpoint = "/blacklist"

type IPVersionFlag int

var IPV4 IPVersionFlag = 4
var IPV6 IPVersionFlag = 6

type BlackListRequest struct {
	client             *Client
	accept             string
	confidenceInterval int
	limit              int
	onlyCountries      []string
	exceptCountries    []string
	ipVersion          *IPVersionFlag
}

func (c *Client) BlackList() *BlackListRequest {
	return &BlackListRequest{
		client: c,
		accept: "text/plain",
	}
}

func (r *BlackListRequest) PlainText() *BlackListRequest {
	r.accept = "text/plain"
	return r
}

func (r *BlackListRequest) JSON() *BlackListRequest {
	r.accept = "application/json"
	return r
}

func (r *BlackListRequest) ConfidenceMinimum(confidence int) *BlackListRequest {
	r.confidenceInterval = confidence
	return r
}

func (r *BlackListRequest) Limit(limit int) *BlackListRequest {
	r.limit = limit
	return r
}

func (r *BlackListRequest) OnlyCountries(countries []string) *BlackListRequest {
	r.onlyCountries = countries
	return r
}

func (r *BlackListRequest) ExceptCountries(countries []string) *BlackListRequest {
	r.exceptCountries = countries
	return r
}

func (r *BlackListRequest) IPVersion(flag IPVersionFlag) *BlackListRequest {
	r.ipVersion = &flag
	return r
}

func (r *BlackListRequest) Fetch() (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+blacklistEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("abuseipdb: creating request: %w", err)
	}

	q := req.URL.Query()
	q.Set("confidenceMinimum", strconv.Itoa(r.confidenceInterval))
	if r.limit > 0 {
		q.Set("limit", strconv.Itoa(r.limit))
	}
	if len(r.onlyCountries) > 0 {
		q.Set("onlyCountries", strings.Join(r.onlyCountries, ","))
	}
	if len(r.exceptCountries) > 0 {
		q.Set("exceptCountries", strings.Join(r.exceptCountries, ","))
	}
	if r.ipVersion != nil {
		q.Set("ipVersion", strconv.Itoa(int(*r.ipVersion)))
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", r.accept)

	return r.client.do(req)
}
