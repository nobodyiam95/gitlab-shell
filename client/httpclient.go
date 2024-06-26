// Package client provides functionality for interacting with HTTP clients
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	socketBaseURL             = "http://unix"
	unixSocketProtocol        = "http+unix://"
	httpProtocol              = "http://"
	httpsProtocol             = "https://"
	defaultReadTimeoutSeconds = 300
	defaultRetryWaitMinimum   = time.Second
	defaultRetryWaitMaximum   = 15 * time.Second
	defaultRetryMax           = 2
)

// ErrCafileNotFound indicates that the specified CA file was not found
var ErrCafileNotFound = errors.New("cafile not found")

// HTTPClient provides an HTTP client with retry capabilities
type HTTPClient struct {
	RetryableHTTP *retryablehttp.Client
	Host          string
}

type httpClientCfg struct {
	keyPath, certPath          string
	caFile, caPath             string
	retryWaitMin, retryWaitMax time.Duration
	retryMax                   int
}

func (hcc httpClientCfg) HaveCertAndKey() bool { return hcc.keyPath != "" && hcc.certPath != "" }

// HTTPClientOpt provides options for configuring an HttpClient
type HTTPClientOpt func(*httpClientCfg)

// WithClientCert will configure the HttpClient to provide client certificates
// when connecting to a server.
func WithClientCert(certPath, keyPath string) HTTPClientOpt {
	return func(hcc *httpClientCfg) {
		hcc.keyPath = keyPath
		hcc.certPath = certPath
	}
}

// WithHTTPRetryOpts configures HTTP retry options for the HttpClient
func WithHTTPRetryOpts(waitMin, waitMax time.Duration, maxAttempts int) HTTPClientOpt {
	return func(hcc *httpClientCfg) {
		hcc.retryWaitMin = waitMin
		hcc.retryWaitMax = waitMax
		hcc.retryMax = maxAttempts
	}
}

func validateCaFile(filename string) error {
	if filename == "" {
		return nil
	}

	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot find cafile '%s': %w", filename, ErrCafileNotFound)
		}

		return err
	}

	return nil
}

// NewHTTPClientWithOpts builds an HTTP client using the provided options
func NewHTTPClientWithOpts(gitlabURL, gitlabRelativeURLRoot, caFile, caPath string, readTimeoutSeconds uint64, opts []HTTPClientOpt) (*HTTPClient, error) {
	hcc := &httpClientCfg{
		caFile:       caFile,
		caPath:       caPath,
		retryWaitMin: defaultRetryWaitMinimum,
		retryWaitMax: defaultRetryWaitMaximum,
		retryMax:     defaultRetryMax,
	}

	for _, opt := range opts {
		opt(hcc)
	}

	var transport *http.Transport
	var host string
	var err error
	switch {
	case strings.HasPrefix(gitlabURL, unixSocketProtocol):
		transport, host = buildSocketTransport(gitlabURL, gitlabRelativeURLRoot)
	case strings.HasPrefix(gitlabURL, httpProtocol):
		transport, host = buildHTTPTransport(gitlabURL)
	case strings.HasPrefix(gitlabURL, httpsProtocol):
		err = validateCaFile(caFile)
		if err != nil {
			return nil, err
		}
		transport, host, err = buildHTTPSTransport(*hcc, gitlabURL)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown GitLab URL prefix")
	}

	c := retryablehttp.NewClient()
	c.RetryMax = hcc.retryMax
	c.RetryWaitMax = hcc.retryWaitMax
	c.RetryWaitMin = hcc.retryWaitMin
	c.Logger = nil
	c.HTTPClient.Transport = NewTransport(transport)
	c.HTTPClient.Timeout = readTimeout(readTimeoutSeconds)

	client := &HTTPClient{RetryableHTTP: c, Host: host}

	return client, nil
}

func buildSocketTransport(gitlabURL, gitlabRelativeURLRoot string) (*http.Transport, string) {
	socketPath := strings.TrimPrefix(gitlabURL, unixSocketProtocol)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	host := socketBaseURL
	gitlabRelativeURLRoot = strings.Trim(gitlabRelativeURLRoot, "/")
	if gitlabRelativeURLRoot != "" {
		host = host + "/" + gitlabRelativeURLRoot
	}

	return transport, host
}

func buildHTTPSTransport(hcc httpClientCfg, gitlabURL string) (*http.Transport, string, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		certPool = x509.NewCertPool()
	}

	if hcc.caFile != "" {
		addCertToPool(certPool, hcc.caFile)
	}

	if hcc.caPath != "" {
		fis, _ := os.ReadDir(hcc.caPath)
		for _, fi := range fis {
			if fi.IsDir() {
				continue
			}

			addCertToPool(certPool, filepath.Join(hcc.caPath, fi.Name()))
		}
	}
	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}

	if hcc.HaveCertAndKey() {
		cert, loadErr := tls.LoadX509KeyPair(hcc.certPath, hcc.keyPath)
		if loadErr != nil {
			return nil, "", loadErr
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return transport, gitlabURL, err
}

func addCertToPool(certPool *x509.CertPool, fileName string) {
	cert, err := os.ReadFile(filepath.Clean(fileName))
	if err == nil {
		certPool.AppendCertsFromPEM(cert)
	}
}

func buildHTTPTransport(gitlabURL string) (*http.Transport, string) {
	return &http.Transport{}, gitlabURL
}

func readTimeout(timeoutSeconds uint64) time.Duration {
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultReadTimeoutSeconds
	}

	return time.Duration(timeoutSeconds) * time.Second
}
