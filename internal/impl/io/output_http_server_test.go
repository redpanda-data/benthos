package io_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

func parseYAMLOutputConf(t testing.TB, formatStr string, args ...any) (conf output.Config) {
	t.Helper()
	var err error
	conf, err = testutil.OutputFromYAML(fmt.Sprintf(formatStr, args...))
	require.NoError(t, err)
	return
}

func TestHTTPServerOutputBasic(t *testing.T) {
	ctx, done := context.WithTimeout(context.Background(), time.Second*30)
	defer done()

	nTestLoops := 10

	port := getFreePort(t)
	conf := parseYAMLOutputConf(t, `
http_server:
  address: localhost:%v
  path: /testpost
`, port)

	h, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	msgChan := make(chan message.Transaction)
	resChan := make(chan error)

	if err = h.Consume(msgChan); err != nil {
		t.Error(err)
		return
	}
	if err = h.Consume(msgChan); err == nil {
		t.Error("Expected error from double listen")
	}

	<-time.After(time.Millisecond * 100)

	// Test both single and multipart messages.
	for i := 0; i < nTestLoops; i++ {
		testStr := fmt.Sprintf("test%v", i)

		go func() {
			testMsg := message.QuickBatch([][]byte{[]byte(testStr)})
			select {
			case msgChan <- message.NewTransaction(testMsg, resChan):
			case <-time.After(time.Second):
				t.Error("Timed out waiting for message")
				return
			}
			select {
			case resMsg := <-resChan:
				if resMsg != nil {
					t.Error(resMsg)
				}
			case <-time.After(time.Second):
				t.Error("Timed out waiting for response")
			}
		}()

		res, err := http.Get(fmt.Sprintf("http://localhost:%v/testpost", port))
		if err != nil {
			t.Error(err)
			return
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("Wrong error code returned: %v", res.StatusCode)
			return
		}
	}

	h.TriggerCloseNow()
	require.NoError(t, h.WaitForClose(ctx))
}

func TestHTTPServerOutputBadRequests(t *testing.T) {
	ctx, done := context.WithTimeout(context.Background(), time.Second*30)
	defer done()

	port := getFreePort(t)
	conf := parseYAMLOutputConf(t, `
http_server:
  address: localhost:%v
  path: /testpost
`, port)

	h, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	msgChan := make(chan message.Transaction)

	if err = h.Consume(msgChan); err != nil {
		t.Error(err)
		return
	}

	<-time.After(time.Millisecond * 100)

	h.TriggerCloseNow()
	require.NoError(t, h.WaitForClose(ctx))

	_, err = http.Get(fmt.Sprintf("http://localhost:%v/testpost", port))
	if err == nil {
		t.Error("request success when service should be closed")
	}
}

func TestHTTPServerOutputTimeout(t *testing.T) {
	ctx, done := context.WithTimeout(context.Background(), time.Second*30)
	defer done()

	port := getFreePort(t)
	conf := parseYAMLOutputConf(t, `
http_server:
  address: localhost:%v
  path: /testpost
  timeout: 1ms
`, port)

	h, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	msgChan := make(chan message.Transaction)

	if err = h.Consume(msgChan); err != nil {
		t.Error(err)
		return
	}

	<-time.After(time.Millisecond * 100)

	var res *http.Response
	res, err = http.Get(fmt.Sprintf("http://localhost:%v/testpost", port))
	if err != nil {
		t.Error(err)
		return
	}
	if exp, act := http.StatusRequestTimeout, res.StatusCode; exp != act {
		t.Errorf("Unexpected status code: %v != %v", exp, act)
	}

	h.TriggerCloseNow()
	require.NoError(t, h.WaitForClose(ctx))
}

func TestHTTPServerOutputTLS(t *testing.T) {
	ctx, done := context.WithTimeout(context.Background(), time.Second*30)
	defer done()

	nTestLoops := 10

	certFile, keyFile, caCert, err := createCertFiles()
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(certFile.Name())
		os.Remove(keyFile.Name())
	})

	port := getFreePort(t)
	conf := parseYAMLOutputConf(t, `
http_server:
  address: localhost:%v
  path: /testpost
  tls:
    enabled: true
    server_certs:
      - cert_file: %s
        key_file: %s
`, port, certFile.Name(), keyFile.Name())

	h, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	msgChan := make(chan message.Transaction)
	resChan := make(chan error)

	if err = h.Consume(msgChan); err != nil {
		t.Error(err)
		return
	}

	<-time.After(time.Millisecond * 100)

	// Test both single and multipart messages.
	for i := 0; i < nTestLoops; i++ {
		testStr := fmt.Sprintf("test%v", i)

		go func() {
			testMsg := message.QuickBatch([][]byte{[]byte(testStr)})
			select {
			case msgChan <- message.NewTransaction(testMsg, resChan):
			case <-time.After(time.Second):
				t.Error("Timed out waiting for message")
				return
			}
			select {
			case resMsg := <-resChan:
				if resMsg != nil {
					t.Error(resMsg)
				}
			case <-time.After(time.Second):
				t.Error("Timed out waiting for response")
			}
		}()

		rootCA := x509.NewCertPool()
		rootCA.AddCert(caCert)
		httpClient := http.DefaultClient
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootCA,
			},
		}

		res, err := httpClient.Get(fmt.Sprintf("https://localhost:%v/testpost", port))
		if err != nil {
			t.Error(err)
			return
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("Wrong error code returned: %v", res.StatusCode)
			return
		}
	}

	h.TriggerCloseNow()
	require.NoError(t, h.WaitForClose(ctx))
}
