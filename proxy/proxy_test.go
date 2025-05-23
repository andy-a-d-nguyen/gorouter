package proxy_test

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry/dropsonde/factories"
	"github.com/cloudfoundry/sonde-go/events"
	uuid "github.com/nu7hatch/gouuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/openzipkin/zipkin-go/propagation/b3"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/websocket"

	"code.cloudfoundry.org/gorouter/common/health"
	router_http "code.cloudfoundry.org/gorouter/common/http"
	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Proxy", func() {
	Describe("Supported HTTP Protocol Versions", func() {
		It("responds to http/1.0", func() {
			ln := test_util.RegisterConnHandler(r, "test", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET / HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET / HTTP/1.0",
				"Host: test",
			})

			conn.CheckLine("HTTP/1.0 200 OK")
		})

		It("responds to HTTP/1.1", func() {
			ln := test_util.RegisterConnHandler(r, "test", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET / HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET / HTTP/1.1",
				"Host: test",
			})

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		Describe("proxying HTTP2", func() {
			var (
				registerConfig    test_util.RegisterConfig
				gorouterCertChain test_util.CertChain
			)

			BeforeEach(func() {
				serverCertDomainSAN := "some-domain-uuid"
				var err error
				caCertPool, err = x509.SystemCertPool()
				Expect(err).NotTo(HaveOccurred())
				backendCACertPool := x509.NewCertPool()

				backendCertChain := test_util.CreateCertAndAddCA(test_util.CertNames{
					CommonName: serverCertDomainSAN,
					SANs:       test_util.SubjectAltNames{IP: "127.0.0.1"},
				}, caCertPool)

				gorouterCertChain = test_util.CreateCertAndAddCA(test_util.CertNames{
					CommonName: "gorouter",
					SANs:       test_util.SubjectAltNames{IP: "127.0.0.1"},
				}, backendCACertPool)

				backendTLSConfig := backendCertChain.AsTLSConfig()
				backendTLSConfig.ClientCAs = backendCACertPool
				backendTLSConfig.NextProtos = []string{"h2", "http/1.1"}

				conf.Backends.ClientAuthCertificate, err = tls.X509KeyPair(gorouterCertChain.CertPEM, gorouterCertChain.PrivKeyPEM)
				Expect(err).NotTo(HaveOccurred())

				registerConfig = test_util.RegisterConfig{
					Protocol:   "http2",
					TLSConfig:  backendTLSConfig,
					InstanceId: "instance-1",
					AppId:      "app-1",
				}
			})

			Context("when HTTP/2 is disabled", func() {
				BeforeEach(func() {
					conf.EnableHTTP2 = false
				})

				It("does NOT issue HTTP/2 requests to backends configured with 'http2' protocol", func() {
					ln := test_util.RegisterHTTPHandler(r, "test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						Expect(r.Proto).To(Equal("HTTP/1.1"))

						w.WriteHeader(http.StatusOK)
					}), registerConfig)
					defer ln.Close()

					client := &http.Client{}

					req, err := http.NewRequest("GET", "http://"+proxyServer.Addr().String(), nil)
					Expect(err).NotTo(HaveOccurred())

					req.Host = "test"

					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(resp.Proto).To(Equal("HTTP/1.1"))
				})
			})

			Context("when HTTP/2 is enabled", func() {
				BeforeEach(func() {
					conf.EnableHTTP2 = true
				})

				It("can proxy inbound HTTP/2 requests to the backend over HTTP/2", func() {
					ln := test_util.RegisterHTTPHandler(r, "test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						Expect(r.Proto).To(Equal("HTTP/2.0"))

						w.WriteHeader(http.StatusOK)
					}), registerConfig)
					defer ln.Close()

					rootCACertPool := x509.NewCertPool()
					rootCACertPool.AddCert(gorouterCertChain.CACert)
					tlsCert, err := tls.X509KeyPair(gorouterCertChain.CACertPEM, gorouterCertChain.CAPrivKeyPEM)
					Expect(err).NotTo(HaveOccurred())

					client := &http.Client{
						Transport: &http2.Transport{
							TLSClientConfig: &tls.Config{
								Certificates: []tls.Certificate{tlsCert},
								RootCAs:      rootCACertPool,
							},
						},
					}

					req, err := http.NewRequest("GET", "https://"+proxyServer.Addr().String(), nil)
					Expect(err).NotTo(HaveOccurred())

					req.Host = "test"

					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(resp.Proto).To(Equal("HTTP/2.0"))
				})
			})
		})

		It("does not respond to unsupported HTTP versions", func() {
			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET / HTTP/1.5",
				"Host: test",
			})

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("URL Handling", func() {
		It("responds transparently to a trailing slash versus no trailing slash", func() {
			lnWithoutSlash := test_util.RegisterConnHandler(r, "test/my%20path/your_path", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET /my%20path/your_path/ HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer lnWithoutSlash.Close()

			lnWithSlash := test_util.RegisterConnHandler(r, "test/another-path/your_path/", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET /another-path/your_path HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer lnWithSlash.Close()

			conn := dialProxy(proxyServer)
			y := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "test", "/my%20path/your_path/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			req = test_util.NewRequest("GET", "test", "/another-path/your_path", nil)
			y.WriteRequest(req)

			resp, _ = y.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("does not append ? to the request", func() {
			ln := test_util.RegisterConnHandler(r, "test/?", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET /? HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			x := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "test", "/?", nil)
			x.WriteRequest(req)
			resp, _ := x.ReadResponse()
			Expect(resp.StatusCode).To(Equal(200))
		})

		It("responds to http/1.0 with path", func() {
			ln := test_util.RegisterConnHandler(r, "test/my_path", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET /my_path HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET /my_path HTTP/1.0",
				"Host: test",
			})

			conn.CheckLine("HTTP/1.0 200 OK")
		})

		It("responds to http/1.0 with path/path", func() {
			ln := test_util.RegisterConnHandler(r, "test/my%20path/your_path", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET /my%20path/your_path HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET /my%20path/your_path HTTP/1.0",
				"Host: test",
			})

			conn.CheckLine("HTTP/1.0 200 OK")
		})

		It("responds to HTTP/1.1 with absolute-form request target", func() {
			ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET http://test.io/ HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET http://test.io/ HTTP/1.1",
				"Host: test.io",
			})

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("responds to http/1.1 with absolute-form request that has encoded characters in the path", func() {
			ln := test_util.RegisterConnHandler(r, "test.io/my%20path/your_path", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET http://test.io/my%20path/your_path HTTP/1.1")

				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			conn.WriteLines([]string{
				"GET http://test.io/my%20path/your_path HTTP/1.1",
				"Host: test.io",
			})

			conn.CheckLine("HTTP/1.1 200 OK")
		})

		It("maintains percent-encoded values in URLs", func() {
			shouldEcho("/abc%2b%2f%25%20%22%3F%5Edef", "/abc%2b%2f%25%20%22%3F%5Edef") // +, /, %, <space>, ", £, ^
		})

		It("does not encode reserved characters in URLs", func() {
			rfc3986_reserved_characters := "!*'();:@&=+$,/?#[]"
			shouldEcho("/"+rfc3986_reserved_characters, "/"+rfc3986_reserved_characters)
		})

		It("maintains encoding of percent-encoded reserved characters", func() {
			encoded_reserved_characters := "%21%27%28%29%3B%3A%40%26%3D%2B%24%2C%2F%3F%23%5B%5D"
			shouldEcho("/"+encoded_reserved_characters, "/"+encoded_reserved_characters)
		})

		It("does not encode unreserved characters in URLs", func() {
			shouldEcho("/abc123_.~def", "/abc123_.~def")
		})

		It("does not percent-encode special characters in URLs (they came in like this, they go out like this)", func() {
			shouldEcho("/abc\"£^def", "/abc\"£^def")
		})

		It("handles requests with encoded query strings", func() {
			queryString := strings.Join([]string{"a=b", url.QueryEscape("b= bc "), url.QueryEscape("c=d&e")}, "&")
			shouldEcho("/test?a=b&b%3D+bc+&c%3Dd%26e", "/test?"+queryString)
		})

		Describe("handling double slashes (//)", func() {
			It("treats double slashes in request URI as an absolute-form request target", func() {
				ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET http://test.io//something.io HTTP/1.1")
					conn.WriteResponse(test_util.NewResponse(http.StatusOK))
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				conn.WriteLines([]string{
					"GET //something.io HTTP/1.1",
					"Host: test.io",
				})

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("handles double slashes in an absolute-form request target correctly", func() {
				ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET http://test.io//something?q=something HTTP/1.1")
					conn.WriteResponse(test_util.NewResponse(http.StatusOK))
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				conn.WriteLines([]string{
					"GET //something?q=something HTTP/1.1",
					"Host: test.io",
				})

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("transparently preserves the multiple slashes", func() {
				ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET http://test.io//something.io//path///to/something HTTP/1.1")
					conn.WriteResponse(test_util.NewResponse(http.StatusOK))
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				conn.WriteLines([]string{
					"GET //something.io//path///to/something HTTP/1.1",
					"Host: test.io",
				})

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

			})

			It("escapes query parameters", func() {
				ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET http://test.io//path?q=something%0A HTTP/1.1")
					conn.WriteResponse(test_util.NewResponse(http.StatusOK))
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				conn.WriteLines([]string{
					"GET //path?q=something%0a HTTP/1.1",
					"Host: test.io",
				})

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("escapes every part of the path", func() {
				ln := test_util.RegisterConnHandler(r, "test.io", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET http://test.io//path%0A/to%0A//something%0A HTTP/1.1")
					conn.WriteResponse(test_util.NewResponse(http.StatusOK))
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				conn.WriteLines([]string{
					"GET //path%0a/to%0a//something%0a HTTP/1.1",
					"Host: test.io",
				})

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
		})
	})

	Describe("proxying the request headers", func() {
		var (
			receivedHeaders  chan http.Header
			extraRegisterCfg []test_util.RegisterConfig
			fakeResponseBody string
			fakeResponseCode int
			ln               net.Listener
			req              *http.Request
		)

		BeforeEach(func() {
			receivedHeaders = make(chan http.Header)
			extraRegisterCfg = nil
			fakeResponseBody = ""
			fakeResponseCode = http.StatusOK
		})

		JustBeforeEach(func() {
			ln = test_util.RegisterConnHandler(r, "app", func(conn *test_util.HttpConn) {
				tmpReq, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(fakeResponseCode)
				conn.WriteResponse(resp)
				if fakeResponseBody != "" {
					conn.WriteLine(fakeResponseBody)
				}
				conn.Close()

				receivedHeaders <- tmpReq.Header
			}, extraRegisterCfg...)

			req = test_util.NewRequest("GET", "app", "/", nil)
		})

		AfterEach(func() {
			ln.Close()
		})

		// proxies request, returns the value of the X-Forwarded-Proto header
		getProxiedHeaders := func(req *http.Request) http.Header {
			conn := dialProxy(proxyServer)
			conn.WriteRequest(req)
			defer conn.ReadResponse()

			var headers http.Header
			Eventually(receivedHeaders).Should(Receive(&headers))
			return headers
		}

		Describe("X-Forwarded-For", func() {
			It("sets X-Forwarded-For", func() {
				Expect(getProxiedHeaders(req).Get("X-Forwarded-For")).To(Equal("127.0.0.1"))
			})
			Context("when the header is already set", func() {
				It("appends the client IP", func() {
					req.Header.Add("X-Forwarded-For", "1.2.3.4")
					Expect(getProxiedHeaders(req).Get("X-Forwarded-For")).To(Equal("1.2.3.4, 127.0.0.1"))
				})
			})
		})

		Describe("X-Forwarded-Host", func() {
			Context("for expect-100-continue requests", func() {
				It("preserves the X-Forwarded-Host header", func() {
					req.Header.Add("X-Forwarded-Host", "foobar.com")
					req.Header.Add("Expect", "100-continue")
					Expect(getProxiedHeaders(req).Get("X-Forwarded-Host")).To(Equal("foobar.com"))
				})
			})
		})

		Describe("X-Request-Start", func() {
			It("appends X-Request-Start", func() {
				Expect(getProxiedHeaders(req).Get("X-Request-Start")).To(MatchRegexp("^\\d{10}\\d{3}$")) // unix timestamp millis
			})

			Context("when the header is already set", func() {
				It("does not modify the header", func() {
					req.Header.Add("X-Request-Start", "") // impl cannot just check for empty string
					req.Header.Add("X-Request-Start", "user-set2")
					Expect(getProxiedHeaders(req)["X-Request-Start"]).To(Equal([]string{"", "user-set2"}))
				})
			})
		})

		Describe("X-CF-InstanceID", func() {
			Context("when the instance is registered with an instance id", func() {
				BeforeEach(func() {
					extraRegisterCfg = []test_util.RegisterConfig{{InstanceId: "fake-instance-id"}}
				})
				It("sets the X-CF-InstanceID header", func() {
					Expect(getProxiedHeaders(req).Get(router_http.CfInstanceIdHeader)).To(Equal("fake-instance-id"))
				})
			})

			Context("when the instance is not registered with an explicit instance id", func() {
				It("sets the X-CF-InstanceID header with the backend host:port", func() {
					Expect(getProxiedHeaders(req).Get(router_http.CfInstanceIdHeader)).To(MatchRegexp(`^\d+(\.\d+){3}:\d+$`))
				})
			})
		})

		Describe("Content-type", func() {
			It("does not set the Content-Type header", func() {
				Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
			})

			Context("when the response body is XML", func() {
				BeforeEach(func() {
					fakeResponseBody = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
				})
				It("still does not set the Content-Type header", func() {
					Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
				})
			})

			Context("when the response code is 204", func() {
				BeforeEach(func() {
					fakeResponseCode = http.StatusNoContent
				})
				It("still does not set the Content-Type header", func() {
					Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
				})
			})
		})

		Describe("100-Continue", func() {
			Context("when the request contains 'Expect: 100-Continue' header", func() {
				JustBeforeEach(func() {
					req.Header.Set("Expect", "100-Continue")
				})

				Context("when config has KeepAlive100ContinueRequests set to true", func() {
					BeforeEach(func() {
						conf.KeepAlive100ContinueRequests = true
					})

					It("should not set 'Connection: close'", func() {
						Expect(getProxiedHeaders(req).Get("Connection")).To(BeEmpty())
					})
				})

				Context("when config has KeepAlive100ContinueRequests set to false", func() {
					BeforeEach(func() {
						conf.KeepAlive100ContinueRequests = false
					})

					It("should set 'Connection: close'", func() {
						Expect(getProxiedHeaders(req).Get("Connection")).To(Equal("close"))
					})
					Describe("X-Forwarded-For", func() {
						It("sets X-Forwarded-For", func() {
							Expect(getProxiedHeaders(req).Get("X-Forwarded-For")).To(Equal("127.0.0.1"))
						})
						Context("when the header is already set", func() {
							It("appends the client IP", func() {
								req.Header.Add("X-Forwarded-For", "1.2.3.4")
								Expect(getProxiedHeaders(req).Get("X-Forwarded-For")).To(Equal("1.2.3.4, 127.0.0.1"))
							})
						})
					})

					Describe("X-Request-Start", func() {
						It("appends X-Request-Start", func() {
							Expect(getProxiedHeaders(req).Get("X-Request-Start")).To(MatchRegexp("^\\d{10}\\d{3}$")) // unix timestamp millis
						})

						Context("when the header is already set", func() {
							It("does not modify the header", func() {
								req.Header.Add("X-Request-Start", "") // impl cannot just check for empty string
								req.Header.Add("X-Request-Start", "user-set2")
								Expect(getProxiedHeaders(req)["X-Request-Start"]).To(Equal([]string{"", "user-set2"}))
							})
						})
					})

					Describe("X-CF-InstanceID", func() {
						Context("when the instance is registered with an instance id", func() {
							BeforeEach(func() {
								extraRegisterCfg = []test_util.RegisterConfig{{InstanceId: "fake-instance-id"}}
							})
							It("sets the X-CF-InstanceID header", func() {
								Expect(getProxiedHeaders(req).Get(router_http.CfInstanceIdHeader)).To(Equal("fake-instance-id"))
							})
						})

						Context("when the instance is not registered with an explicit instance id", func() {
							It("sets the X-CF-InstanceID header with the backend host:port", func() {
								Expect(getProxiedHeaders(req).Get(router_http.CfInstanceIdHeader)).To(MatchRegexp(`^\d+(\.\d+){3}:\d+$`))
							})
						})
					})

					Describe("Content-type", func() {
						It("does not set the Content-Type header", func() {
							Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
						})

						Context("when the response body is XML", func() {
							BeforeEach(func() {
								fakeResponseBody = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
							})
							It("still does not set the Content-Type header", func() {
								Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
							})
						})

						Context("when the response code is 204", func() {
							BeforeEach(func() {
								fakeResponseCode = http.StatusNoContent
							})
							It("still does not set the Content-Type header", func() {
								Expect(getProxiedHeaders(req)).NotTo(HaveKey("Content-Type"))
							})
						})
					})
					Describe("X-Forwarded-Client-Cert", func() {
						Context("when gorouter is configured with ForwardedClientCert == sanitize_set", func() {
							BeforeEach(func() {
								conf.ForwardedClientCert = config.SANITIZE_SET
							})
							It("removes xfcc header", func() {
								req.Header.Add("X-Forwarded-Client-Cert", "foo")
								req.Header.Add("X-Forwarded-Client-Cert", "bar")
								Expect(getProxiedHeaders(req).Get("X-Forwarded-Client-Cert")).To(BeEmpty())
							})
						})

						Context("when ForwardedClientCert is set to forward but the request is not mTLS", func() {
							BeforeEach(func() {
								conf.ForwardedClientCert = config.FORWARD
							})
							It("removes xfcc header", func() {
								req.Header.Add("X-Forwarded-Client-Cert", "foo")
								req.Header.Add("X-Forwarded-Client-Cert", "bar")
								Expect(getProxiedHeaders(req).Get("X-Forwarded-Client-Cert")).To(BeEmpty())
							})
						})

						Context("when ForwardedClientCert is set to always_forward", func() {
							BeforeEach(func() {
								conf.ForwardedClientCert = config.ALWAYS_FORWARD
							})
							It("leaves the xfcc header intact", func() {
								req.Header.Add("X-Forwarded-Client-Cert", "foo")
								req.Header.Add("X-Forwarded-Client-Cert", "bar")
								Expect(getProxiedHeaders(req)).To(HaveKeyWithValue("X-Forwarded-Client-Cert", []string{"foo", "bar"}))
							})
						})
					})
				})
			})

			Context("when the request does not contain 'Expect: 100-Continue' header", func() {
				It("should not set 'Connection: close'", func() {
					Expect(getProxiedHeaders(req).Get("Connection")).To(BeEmpty())
				})
			})
		})

		Describe("X-Forwarded-Client-Cert", func() {
			Context("when gorouter is configured with ForwardedClientCert == sanitize_set", func() {
				BeforeEach(func() {
					conf.ForwardedClientCert = config.SANITIZE_SET
				})
				It("removes xfcc header", func() {
					req.Header.Add("X-Forwarded-Client-Cert", "foo")
					req.Header.Add("X-Forwarded-Client-Cert", "bar")
					Expect(getProxiedHeaders(req).Get("X-Forwarded-Client-Cert")).To(BeEmpty())
				})
			})

			Context("when ForwardedClientCert is set to forward but the request is not mTLS", func() {
				BeforeEach(func() {
					conf.ForwardedClientCert = config.FORWARD
				})
				It("removes xfcc header", func() {
					req.Header.Add("X-Forwarded-Client-Cert", "foo")
					req.Header.Add("X-Forwarded-Client-Cert", "bar")
					Expect(getProxiedHeaders(req).Get("X-Forwarded-Client-Cert")).To(BeEmpty())
				})
			})

			Context("when ForwardedClientCert is set to always_forward", func() {
				BeforeEach(func() {
					conf.ForwardedClientCert = config.ALWAYS_FORWARD
				})
				It("leaves the xfcc header intact", func() {
					req.Header.Add("X-Forwarded-Client-Cert", "foo")
					req.Header.Add("X-Forwarded-Client-Cert", "bar")
					Expect(getProxiedHeaders(req)).To(HaveKeyWithValue("X-Forwarded-Client-Cert", []string{"foo", "bar"}))
				})
			})
		})
	})

	Describe("Response Handling", func() {
		It("trace headers added on correct TraceKey", func() {
			ln := test_util.RegisterConnHandler(r, "trace-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "trace-test", "/", nil)
			req.Header.Set(router_http.VcapTraceHeader, "my_trace_key")
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get(router_http.VcapBackendHeader)).To(Equal(ln.Addr().String()))
			Expect(resp.Header.Get(router_http.CfRouteEndpointHeader)).To(Equal(ln.Addr().String()))
			Expect(resp.Header.Get(router_http.VcapRouterHeader)).To(Equal(conf.Ip))
		})

		It("trace headers not added on incorrect TraceKey", func() {
			ln := test_util.RegisterConnHandler(r, "trace-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "trace-test", "/", nil)
			req.Header.Set(router_http.VcapTraceHeader, "a_bad_trace_key")
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.Header.Get(router_http.VcapBackendHeader)).To(Equal(""))
			Expect(resp.Header.Get(router_http.CfRouteEndpointHeader)).To(Equal(""))
			Expect(resp.Header.Get(router_http.VcapRouterHeader)).To(Equal(""))
		})

		It("adds X-Vcap-Request-Id if it doesn't already exist in the response", func() {
			ln := test_util.RegisterConnHandler(r, "vcap-id-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "vcap-id-test", "/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get(handlers.VcapRequestIdHeader)).ToNot(BeEmpty())
		})

		It("does not adds X-Vcap-Request-Id if it already exists in the response", func() {
			ln := test_util.RegisterConnHandler(r, "vcap-id-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				resp.Header.Set(handlers.VcapRequestIdHeader, "foobar")
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "vcap-id-test", "/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get(handlers.VcapRequestIdHeader)).To(Equal("foobar"))
		})

		It("Status No Content returns no Transfer Encoding response header", func() {
			ln := test_util.RegisterConnHandler(r, "not-modified", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusNoContent)
				resp.Header.Set("Connection", "close")
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "not-modified", "/", nil)

			req.Header.Set("Connection", "close")
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			Expect(resp.TransferEncoding).To(BeNil())
		})

		It("transfers chunked encodings", func() {
			ln := test_util.RegisterHTTPHandler(r, "chunk", func(w http.ResponseWriter, r *http.Request) {
				flusher, ok := w.(http.Flusher)
				Expect(ok).To(BeTrue())

				for i := 0; i < 3; i++ {
					_, err := w.Write([]byte("hello"))
					Expect(err).NotTo(HaveOccurred())
					flusher.Flush()
				}
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "chunk", "/", nil)

			err := req.Write(conn)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.ReadResponse(conn.Reader, &http.Request{})
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.TransferEncoding).To(Equal([]string{"chunked"}))

			// Expect 3 individual reads to complete
			b := make([]byte, 5)
			for i := 0; i < 3; i++ {
				n, err := resp.Body.Read(b[0:])
				if err != nil {
					Expect(err).To(Equal(io.EOF))
				}
				Expect(n).To(Equal(5))
				Expect(string(b[0:n])).To(Equal("hello"))
			}
		})

		It("disables compression", func() {
			ln := test_util.RegisterConnHandler(r, "remote", func(conn *test_util.HttpConn) {
				request, _ := http.ReadRequest(conn.Reader)
				encoding := request.Header["Accept-Encoding"]
				var resp *http.Response
				if len(encoding) != 0 {
					resp = test_util.NewResponse(http.StatusInternalServerError)
				} else {
					resp = test_util.NewResponse(http.StatusOK)
				}
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "remote", "/", nil)
			conn.WriteRequest(req)
			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		Context("when concurrent read write is not enabled", func() {
			BeforeEach(func() {
				conf.EnableHTTP1ConcurrentReadWrite = false
			})

			It("can not simultaneously read request and write response", func() {
				contentLength := len("message 0\n") * 10
				ln := test_util.RegisterConnHandler(r, "read-write", func(conn *test_util.HttpConn) {
					defer conn.Close()

					req, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())
					defer req.Body.Close()

					conn.Writer.WriteString("HTTP/1.1 200 OK\r\n" +
						"Connection: keep-alive\r\n" +
						"Content-Type: text/plain\r\n" +
						fmt.Sprintf("Content-Length: %d\r\n", contentLength) +
						"\r\n")
					conn.Writer.Flush()
					reader := bufio.NewReader(req.Body)

					for i := 0; i < 10; i++ {
						// send back the received message
						line, err := reader.ReadString('\n')
						if err != nil {
							break
						}
						conn.Writer.Write([]byte(line))
						conn.Writer.Flush()
					}
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				// the test is hanging when fix is not implemented
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))

				conn.Writer.Write([]byte("GET / HTTP/1.1\r\n" +
					"Host: read-write\r\n" +
					"Connection: keep-alive\r\n" +
					"Content-Type: text/plain\r\n" +
					fmt.Sprintf("Content-Length: %d\r\n", contentLength) +
					"\r\n",
				))
				conn.Writer.Flush()

				_, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when concurrent read write is enabled", func() {
			BeforeEach(func() {
				conf.EnableHTTP1ConcurrentReadWrite = true
			})

			It("can simultaneously read request and write response", func() {
				contentLength := len("message 0\n") * 10
				ln := test_util.RegisterConnHandler(r, "read-write", func(conn *test_util.HttpConn) {
					defer conn.Close()

					req, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())
					defer req.Body.Close()

					conn.Writer.WriteString("HTTP/1.1 200 OK\r\n" +
						"Connection: keep-alive\r\n" +
						"Content-Type: text/plain\r\n" +
						fmt.Sprintf("Content-Length: %d\r\n", contentLength) +
						"\r\n")
					conn.Writer.Flush()
					reader := bufio.NewReader(req.Body)

					for i := 0; i < 10; i++ {
						// send back the received message
						line, err := reader.ReadString('\n')
						if err != nil {
							break
						}
						conn.Writer.Write([]byte(line))
						conn.Writer.Flush()
					}
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				// the test is hanging when fix is not implemented
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))

				conn.Writer.Write([]byte("GET / HTTP/1.1\r\n" +
					"Host: read-write\r\n" +
					"Connection: keep-alive\r\n" +
					"Content-Type: text/plain\r\n" +
					fmt.Sprintf("Content-Length: %d\r\n", contentLength) +
					"\r\n",
				))
				conn.Writer.Flush()

				resp, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				reader := bufio.NewReader(resp.Body)

				for i := 0; i < 10; i++ {
					message := fmt.Sprintf("message %d\n", i)
					conn.Writer.Write([]byte(message))
					conn.Writer.Flush()
					line, err := reader.ReadString('\n')
					Expect(err).NotTo(HaveOccurred())
					Expect(line).To(Equal(message))
				}
			})
		})

		It("retries on POST requests if nothing was written", func() {
			//FIXME: this test is flakey. we can't skip/pend this test since CI will fail on any skipped/pending
			//       tests
			//
			// We believe the flakiness in this test was introduce in commit 4b605dd344b3546ec3d70c1e1ed5acd376ebae11
			// but we're unsure of the ramifications of reverting the commit. We are working on a fix.
			//
			// May 25, 2023

			bad1 := test_util.RegisterConnHandler(r, "retry-test", func(conn *test_util.HttpConn) {
				conn.Close()
			})
			defer bad1.Close()
			bad2 := test_util.RegisterConnHandler(r, "retry-test", func(conn *test_util.HttpConn) {
				conn.Close()
			})
			defer bad2.Close()
			good := test_util.RegisterConnHandler(r, "retry-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer good.Close()

			// FIXME: This used to be Consistently, but moved to eventually to work around flakiness in this test
			Eventually(func() int {
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("POST", "retry-test", "/", strings.NewReader("some body"))
				conn.WriteRequest(req)

				resp, _ := conn.ReadResponse()
				return resp.StatusCode
			}, 2*time.Second, 100*time.Millisecond).Should(Equal(http.StatusOK))
		})

	})

	Describe("HTTP Rewrite", func() {
		mockedHandler := func(host string, headers []string) net.Listener {
			return test_util.RegisterConnHandler(r, host, func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				for _, h := range headers {
					resp.Header.Set(strings.Split(h, ":")[0], strings.Split(h, ":")[1])
				}
				conn.WriteResponse(resp)
				conn.Close()
			})
		}

		process := func(host string) *http.Response {
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", host, "/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			return resp
		}

		It("does not add a rewrite handler if not configured", func() {
			ln := mockedHandler("hsts-test", []string{})
			defer ln.Close()

			process("hsts-test")
			Expect(logger).NotTo(gbytes.Say("http-rewrite"))
		})

		Context("when add response header is set", func() {
			BeforeEach(func() {
				conf.HTTPRewrite = config.HTTPRewrite{
					Responses: config.HTTPRewriteResponses{
						AddHeadersIfNotPresent: []config.HeaderNameValue{
							{Name: "X-Foo", Value: "bar"},
						},
					},
				}
			})

			It("adds the header if it doesn't already exist in the response", func() {
				ln := mockedHandler("hsts-test", []string{})
				defer ln.Close()

				header := process("hsts-test").Header
				Expect(header.Get("X-Foo")).To(Equal("bar"))
			})

			It("does not add the header if it already exists in the response", func() {
				ln := mockedHandler("hsts-test", []string{"X-Foo: foo"})
				defer ln.Close()

				header := process("hsts-test").Header
				Expect(header.Get("X-Foo")).To(Equal("foo"))
			})

			It("adds the header for unknown routes", func() {
				ln := mockedHandler("hsts-test", []string{})
				defer ln.Close()

				header := process("other-host").Header
				Expect(header.Get("X-Foo")).To(Equal("bar"))
			})
		})

		Context("when remove response header is set", func() {
			BeforeEach(func() {
				conf.HTTPRewrite = config.HTTPRewrite{
					Responses: config.HTTPRewriteResponses{
						RemoveHeaders: []config.HeaderNameValue{
							{Name: "X-Vcap-Request-Id"},
							{Name: "X-Foo"},
						},
					},
				}
			})

			It("can remove headers set by gorouter like X-Vcap-Request-Id", func() {
				ln := mockedHandler("hsts-test", []string{})
				defer ln.Close()

				header := process("hsts-test").Header
				Expect(header.Get(handlers.VcapRequestIdHeader)).To(BeEmpty())
			})

			It("removes the headers that match and only those", func() {
				ln := mockedHandler("hsts-test", []string{"X-Foo: foo", "X-Bar: bar"})
				defer ln.Close()

				header := process("hsts-test").Header
				Expect(header).ToNot(HaveKey("foo"))
				Expect(header.Get("X-Bar")).To(Equal("bar"))
			})
		})
	})

	Describe("Backend Connection Handling", func() {
		Context("when max conn per backend is set to > 0 ", func() {
			BeforeEach(func() {
				conf.Backends.MaxConns = 2
			})

			It("responds with 503 after conn limit is reached ", func() {
				ln := test_util.RegisterConnHandler(r, "sleep", func(x *test_util.HttpConn) {
					defer GinkgoRecover()
					_, err := http.ReadRequest(x.Reader)
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(50 * time.Millisecond)
					resp := test_util.NewResponse(http.StatusOK)
					x.WriteResponse(resp)
					x.WriteLine("hello from server after sleeping")
					x.Close()
				})
				defer ln.Close()

				var wg sync.WaitGroup
				var badGatewayCount int32

				for i := 0; i < 3; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer GinkgoRecover()

						x := dialProxy(proxyServer)
						defer x.Close()

						req := test_util.NewRequest("GET", "sleep", "/", nil)
						req.Host = "sleep"

						x.WriteRequest(req)
						resp, _ := x.ReadResponse()
						if resp.StatusCode == http.StatusServiceUnavailable {
							atomic.AddInt32(&badGatewayCount, 1)
						} else if resp.StatusCode != http.StatusOK {
							Fail(fmt.Sprintf("Expected resp to return 200 or 503, got %d", resp.StatusCode))
						}
					}()
					time.Sleep(10 * time.Millisecond)
				}
				wg.Wait()
				Expect(atomic.LoadInt32(&badGatewayCount)).To(Equal(int32(1)))
			})
		})

		It("request terminates with slow response", func() {
			ln := test_util.RegisterConnHandler(r, "slow-app", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(2 * time.Second)
				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "slow-app", "/", nil)

			started := time.Now()
			conn.WriteRequest(req)

			resp, err := http.ReadResponse(conn.Reader, &http.Request{})
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(time.Since(started)).To(BeNumerically("<", time.Duration(2*time.Second)))
		})

		It("proxy closes connections with slow apps", func() {
			serverResult := make(chan error)
			ln := test_util.RegisterConnHandler(r, "slow-app", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET / HTTP/1.1")

				timesToTick := 5
				// sleep to force a dial timeout
				time.Sleep(1100 * time.Millisecond)

				conn.WriteLines([]string{
					"HTTP/1.1 200 OK",
					fmt.Sprintf("Content-Length: %d", timesToTick),
				})

				for i := 0; i < timesToTick; i++ {
					_, err := conn.Conn.Write([]byte("x"))
					// expect an error due to closed connection
					if err != nil {
						serverResult <- err
						return
					}

					time.Sleep(100 * time.Millisecond)
				}

				serverResult <- nil
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "slow-app", "/", nil)

			started := time.Now()
			conn.WriteRequest(req)

			resp, err := http.ReadResponse(conn.Reader, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(time.Since(started)).To(BeNumerically("<", time.Duration(2*time.Second)))

			// var err error
			Eventually(serverResult, "2s").Should(Receive(&err))
			Expect(err).NotTo(BeNil())
		})

		It("proxy detects closed client connection", func() {
			serverResult := make(chan error)
			readRequest := make(chan struct{})
			ln := test_util.RegisterConnHandler(r, "slow-app", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET / HTTP/1.1")

				readRequest <- struct{}{}

				timesToTick := 10

				conn.WriteLines([]string{
					"HTTP/1.1 200 OK",
					fmt.Sprintf("Content-Length: %d", timesToTick),
				})

				for i := 0; i < timesToTick; i++ {
					_, err := conn.Conn.Write([]byte("x"))
					if err != nil {
						serverResult <- err
						return
					}

					time.Sleep(100 * time.Millisecond)
				}

				serverResult <- nil
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "slow-app", "/", nil)

			conn.WriteRequest(req)
			Eventually(readRequest).Should(Receive())
			conn.Conn.Close()

			var err error
			Eventually(serverResult).Should(Receive(&err))
			Expect(err).NotTo(BeNil())
		})

		It("proxy closes connections to backends when client closes the connection", func() {
			serverResult := make(chan error)
			readRequest := make(chan struct{})
			ln := test_util.RegisterConnHandler(r, "slow-app", func(conn *test_util.HttpConn) {
				conn.CheckLine("GET / HTTP/1.1")

				readRequest <- struct{}{}

				time.Sleep(600 * time.Millisecond)

				for i := 0; i < 2; i++ {
					_, err := conn.Conn.Write([]byte("x"))
					if err != nil {
						serverResult <- err
						return
					}

					time.Sleep(100 * time.Millisecond)
				}

				serverResult <- nil
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "slow-app", "/", nil)

			conn.WriteRequest(req)
			Eventually(readRequest).Should(Receive())
			conn.Conn.Close()

			var err error
			Eventually(serverResult).Should(Receive(&err))
			Expect(err).NotTo(BeNil())
		})

		It("retries when failed endpoints exist", func() {
			ln := test_util.RegisterConnHandler(r, "retries", func(conn *test_util.HttpConn) {
				req, _ := conn.ReadRequest()
				Expect(req.Method).To(Equal("GET"))
				Expect(req.Host).To(Equal("retries"))
				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			test_util.RegisterAddr(r, "retries", "localhost:81", test_util.RegisterConfig{
				InstanceId:    "instanceId",
				InstanceIndex: "2",
			})

			for i := 0; i < 5; i++ {
				body := &bytes.Buffer{}
				body.WriteString("use an actual body")

				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "retries", "/", io.NopCloser(body))
				conn.WriteRequest(req)
				resp, _ := conn.ReadResponse()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			}
		})

		Context("when a TLS handshake occurs", func() {
			var nl net.Listener
			var backendCert tls.Certificate
			BeforeEach(func() {
				certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{CommonName: "instance-id"})
				var err error
				backendCert, err = tls.X509KeyPair(certChain.CertPEM, certChain.PrivKeyPEM)
				Expect(err).NotTo(HaveOccurred())

				caCertPool = x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(certChain.CACertPEM)
			})
			JustBeforeEach(func() {
				nl = test_util.RegisterConnHandler(r, "backend-with-different-instance-id", func(conn *test_util.HttpConn) {
					_, err := http.ReadRequest(conn.Reader)
					Expect(err).To(HaveOccurred())
					resp := test_util.NewResponse(http.StatusServiceUnavailable)
					conn.WriteResponse(resp)
					conn.Close()
				}, test_util.RegisterConfig{
					ServerCertDomainSAN: "a-different-instance-id",
					InstanceId:          "a-different-instance-id",
					AppId:               "some-app-id",
					TLSConfig: &tls.Config{
						Certificates: []tls.Certificate{backendCert},
					},
				})

			})

			AfterEach(func() {
				nl.Close()
			})

			Context("when the server cert does not match the client", func() {
				Context("when emptyPoolResponseCode503 is true", func() {
					BeforeEach(func() {
						conf.EmptyPoolResponseCode503 = true
					})

					It("prunes the route", func() {
						for _, status := range []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable} {
							body := &bytes.Buffer{}
							body.WriteString("use an actual body")
							conn := dialProxy(proxyServer)
							req := test_util.NewRequest("GET", "backend-with-different-instance-id", "/", io.NopCloser(body))
							conn.WriteRequest(req)
							resp, _ := conn.ReadResponse()
							Expect(resp.StatusCode).To(Equal(status))
						}
					})

					Context("when MaxConns is > 0", func() {
						BeforeEach(func() {
							conf.Backends.MaxConns = 2
						})

						It("prunes the route", func() {
							for _, status := range []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable} {
								body := &bytes.Buffer{}
								body.WriteString("use an actual body")
								conn := dialProxy(proxyServer)
								req := test_util.NewRequest("GET", "backend-with-different-instance-id", "/", io.NopCloser(body))
								conn.WriteRequest(req)
								resp, _ := conn.ReadResponse()
								Expect(resp.StatusCode).To(Equal(status))
							}
						})
					})
				})

				Context("when emptyPoolResponseCode503 is false", func() {
					BeforeEach(func() {
						conf.EmptyPoolResponseCode503 = false
					})

					It("prunes the route", func() {
						for _, status := range []int{http.StatusServiceUnavailable, http.StatusNotFound} {
							body := &bytes.Buffer{}
							body.WriteString("use an actual body")
							conn := dialProxy(proxyServer)
							req := test_util.NewRequest("GET", "backend-with-different-instance-id", "/", io.NopCloser(body))
							conn.WriteRequest(req)
							resp, _ := conn.ReadResponse()
							Expect(resp.StatusCode).To(Equal(status))
						}
					})
				})
			})
		})

		Context("when TLS handshake is not reciprocated by the application", func() {
			var nl net.Listener
			JustBeforeEach(func() {
				certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{CommonName: "instance-id"})
				backendCert, err := tls.X509KeyPair(certChain.CertPEM, certChain.PrivKeyPEM)
				Expect(err).NotTo(HaveOccurred())

				caCertPool = x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(certChain.CACertPEM)

				nl, err = net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())

				go func() {
					defer GinkgoRecover()
					for {
						conn, err := nl.Accept()
						if err != nil {
							break
						}
						go func() {
							defer GinkgoRecover()

							httpConn := test_util.NewHttpConn(conn)
							time.Sleep(time.Minute)
							_, err := http.ReadRequest(httpConn.Reader)
							Expect(err).To(HaveOccurred())
							resp := test_util.NewResponse(http.StatusServiceUnavailable)
							httpConn.WriteResponse(resp)
							httpConn.Close()
						}()
					}
				}()

				rcfg := test_util.RegisterConfig{
					ServerCertDomainSAN: "a-different-instance-id",
					InstanceId:          "a-different-instance-id",
					AppId:               "some-app-id",
					InstanceIndex:       "2",
					StaleThreshold:      120,
					TLSConfig: &tls.Config{
						Certificates: []tls.Certificate{backendCert},
					},
				}

				test_util.RegisterAddr(r, "backend-with-tcp-only", nl.Addr().String(), rcfg)
			})

			AfterEach(func() {
				nl.Close()
			})

			It("prunes the route", func() {
				for _, status := range []int{http.StatusBadGateway, http.StatusNotFound} {
					body := &bytes.Buffer{}
					body.WriteString("use an actual body")
					conn := dialProxy(proxyServer)
					req := test_util.NewRequest("GET", "backend-with-tcp-only", "/", io.NopCloser(body))
					conn.WriteRequest(req)
					resp, _ := conn.ReadResponse()
					Expect(resp.StatusCode).To(Equal(status))
				}
			})
		})
	})

	Describe("QueryParam", func() {
		It("logs requests with semicolons in them", func() {
			ln := test_util.RegisterConnHandler(r, "query-param-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "query-param-test", "/?param1;param2", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(logger.Lines(zap.WarnLevel)).To(ContainElement(ContainSubstring("deprecated-semicolon-params")))

		})
	})
	Describe("Access Logging", func() {
		It("logs a request", func() {
			ln := test_util.RegisterConnHandler(r, "test", func(conn *test_util.HttpConn) {
				req, body := conn.ReadRequest()
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/"))
				Expect(req.ProtoMajor).To(Equal(1))
				Expect(req.ProtoMinor).To(Equal(1))

				Expect(body).To(Equal("ABCD"))

				rsp := test_util.NewResponse(200)
				rsp.Body = io.NopCloser(strings.NewReader("DEFG"))
				conn.WriteResponse(rsp)

				conn.Close()
			}, test_util.RegisterConfig{InstanceId: "123", AppId: "456"})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			body := &bytes.Buffer{}
			body.WriteString("ABCD")
			req := test_util.NewRequest("POST", "test", "/", body)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Eventually(func() (int64, error) {
				fi, err := f.Stat()
				if err != nil {
					return 0, err
				}
				return fi.Size(), nil
			}).ShouldNot(BeZero())

			//make sure the record includes all the data
			//since the building of the log record happens throughout the life of the request
			b, err := os.ReadFile(f.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.HasPrefix(string(b), "test - [")).To(BeTrue())
			Expect(string(b)).To(ContainSubstring(`"POST / HTTP/1.1" 200 4 4 "-"`))
			Expect(string(b)).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
			Expect(string(b)).To(ContainSubstring(`response_time:`))
			Expect(string(b)).To(ContainSubstring(`app_id:"456"`))
			Expect(string(b)).To(ContainSubstring(`app_index:"2"`))
			Expect(string(b[len(b)-1])).To(Equal("\n"))
		})

		It("logs a websocket request", func() {
			ln := test_util.RegisterWSHandler(r, "ws-test", func(conn *websocket.Conn) {
				msgBuf := make([]byte, 100)
				n, err := conn.Read(msgBuf)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(msgBuf[:n])).To(Equal("HELLO WEBSOCKET"))

				_, _ = conn.Write([]byte("WEBSOCKET OK"))
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)
			wsConn, err := wsClient(conn, "ws://ws-test")
			Expect(err).NotTo(HaveOccurred())

			_, err = wsConn.Write([]byte("HELLO WEBSOCKET"))
			Expect(err).NotTo(HaveOccurred())

			msgBuf := make([]byte, 100)
			n, err := wsConn.Read(msgBuf)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(msgBuf[:n])).To(Equal("WEBSOCKET OK"))

			Eventually(func() (int64, error) {
				fi, err := f.Stat()
				if err != nil {
					return 0, err
				}
				return fi.Size(), nil
			}).ShouldNot(BeZero())

			//make sure the record includes all the data
			//since the building of the log record happens throughout the life of the request
			logBytes, err := os.ReadFile(f.Name())
			Expect(err).NotTo(HaveOccurred())
			logStr := string(logBytes)
			Expect(strings.HasPrefix(logStr, "ws-test - [")).To(BeTrue())
			Expect(logStr).To(ContainSubstring(`"GET / HTTP/1.1" 101`))
			Expect(logStr).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
			Expect(logStr).To(ContainSubstring(`response_time:`))
			Expect(logStr).To(ContainSubstring(`gorouter_time:`))
		})

		Context("A slow response body", func() {
			BeforeEach(func() {
				conf.EndpointTimeout = 5 * time.Second
			})

			It("shows in response_time, not gorouter_time", func() {
				ln := test_util.RegisterConnHandler(r, "slow-response-app", func(conn *test_util.HttpConn) {
					_, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())

					body := "this body took some time."
					conn.WriteLines([]string{
						"HTTP/1.1 200 OK",
						fmt.Sprintf("Content-Length: %d", len(body)),
					})

					time.Sleep(100 * time.Millisecond)

					conn.WriteLine(body)
					conn.Close()
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "slow-response-app", "/", nil)

				started := time.Now()
				conn.WriteRequest(req)

				resp, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}, "5s").ShouldNot(BeZero())

				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.HasPrefix(string(b), "slow-response-app - [")).To(BeTrue())
				Expect(string(b)).To(ContainSubstring("response_time:"))
				Expect(string(b)).To(ContainSubstring("gorouter_time:"))
				Expect(string(b)).To(ContainSubstring("response_time:0.1"))
				Expect(string(b)).To(ContainSubstring("gorouter_time:0.0"))
			})

			Context("in websocket requests", func() {
				It("shows slowness in response_time, not gorouter_time", func() {
					ln := test_util.RegisterWSHandler(r, "slow-ws-test", func(conn *websocket.Conn) {
						msgBuf := make([]byte, 100)
						n, err := conn.Read(msgBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(string(msgBuf[:n])).To(Equal("HELLO WEBSOCKET"))

						time.Sleep(100 * time.Millisecond)

						_, _ = conn.Write([]byte("WEBSOCKET OK"))
						conn.Close()
					})
					defer ln.Close()

					conn := dialProxy(proxyServer)
					wsConn, err := wsClient(conn, "ws://slow-ws-test")
					Expect(err).NotTo(HaveOccurred())

					_, err = wsConn.Write([]byte("HELLO WEBSOCKET"))
					Expect(err).NotTo(HaveOccurred())

					msgBuf := make([]byte, 100)
					n, err := wsConn.Read(msgBuf)

					Expect(err).NotTo(HaveOccurred())
					Expect(string(msgBuf[:n])).To(Equal("WEBSOCKET OK"))

					Eventually(func() (int64, error) {
						fi, err := f.Stat()
						if err != nil {
							return 0, err
						}
						return fi.Size(), nil
					}).ShouldNot(BeZero())

					//make sure the record includes all the data
					//since the building of the log record happens throughout the life of the request
					logBytes, err := os.ReadFile(f.Name())
					Expect(err).NotTo(HaveOccurred())
					logStr := string(logBytes)
					Expect(strings.HasPrefix(logStr, "slow-ws-test - [")).To(BeTrue())
					Expect(logStr).To(ContainSubstring(`"GET / HTTP/1.1" 101`))
					Expect(logStr).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
					Expect(logStr).To(ContainSubstring(`response_time:0.1`))
					Expect(logStr).To(ContainSubstring(`gorouter_time:0.0`))
				})
			})

			Context("A slow app with multiple broken endpoints and attempt details logging enabled", func() {
				BeforeEach(func() {
					conf.EndpointDialTimeout = 100 * time.Millisecond
					conf.Logging.ExtraAccessLogFields = []string{"failed_attempts", "failed_attempts_time", "dns_time", "dial_time", "tls_time", "backend_time"}
					conf.DropletStaleThreshold = 1
				})

				It("shows in failed_attempts_time and backend_time, not gorouter_time", func() {

					// Register some broken endpoints to cause retries
					test_util.RegisterAddr(r, "partially-broken-app", "10.255.255.1:1234", test_util.RegisterConfig{InstanceIndex: "1"})
					test_util.RegisterAddr(r, "partially-broken-app", "10.255.255.1:1235", test_util.RegisterConfig{InstanceIndex: "2"})

					ln := test_util.RegisterConnHandler(r, "partially-broken-app", func(conn *test_util.HttpConn) {
						_, err := http.ReadRequest(conn.Reader)
						Expect(err).NotTo(HaveOccurred())

						body := "this body took some time."
						conn.WriteLines([]string{
							"HTTP/1.1 200 OK",
							fmt.Sprintf("Content-Length: %d", len(body)),
						})

						time.Sleep(100 * time.Millisecond)

						conn.WriteLine(body)
						conn.Close()
					}, test_util.RegisterConfig{InstanceIndex: "3"})
					defer ln.Close()

					Eventually(func(g Gomega) {
						conn := dialProxy(proxyServer)
						req := test_util.NewRequest("GET", "partially-broken-app", "/", nil)

						started := time.Now()
						conn.WriteRequest(req)

						resp, err := http.ReadResponse(conn.Reader, &http.Request{})
						g.Expect(err).NotTo(HaveOccurred())

						g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
						g.Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

						g.Eventually(func() (int64, error) {
							fi, err := f.Stat()
							if err != nil {
								return 0, err
							}
							return fi.Size(), nil
						}, "10s").ShouldNot(BeZero())

						logBytes, err := os.ReadFile(f.Name())
						g.Expect(err).NotTo(HaveOccurred())
						logStr := string(logBytes)
						g.Expect(strings.HasPrefix(logStr, "partially-broken-app - [")).To(BeTrue())
						g.Expect(logStr).To(ContainSubstring("response_time:"))
						g.Expect(logStr).To(ContainSubstring("gorouter_time:"))
						g.Expect(logStr).To(ContainSubstring("backend_time:0.1"))         // 0.1 seconds delay from slow backend app
						g.Expect(logStr).To(ContainSubstring("failed_attempts_time:0.2")) // plus 0.2 seconds from dial attempts
						g.Expect(logStr).To(ContainSubstring("response_time:0.3"))        // makes 0.3 seconds total response time
						g.Expect(logStr).To(ContainSubstring("gorouter_time:0.0"))
						g.Expect(logStr).To(ContainSubstring("failed_attempts:2"))
						g.Expect(logStr).To(ContainSubstring(`app_index:"3"`))

					}, "20s").Should(Succeed()) // we don't know which endpoint will be chosen first, so we have to try until sequence 1,2,3 has been hit
				})
				It("shows no backend_time or other attempt details if all endpoints are broken", func() {

					// Register some broken endpoints to cause retries
					test_util.RegisterAddr(r, "fully-broken-app", "10.255.255.1:1234", test_util.RegisterConfig{InstanceIndex: "1"})
					test_util.RegisterAddr(r, "fully-broken-app", "10.255.255.1:1235", test_util.RegisterConfig{InstanceIndex: "2"})
					test_util.RegisterAddr(r, "fully-broken-app", "10.255.255.1:1236", test_util.RegisterConfig{InstanceIndex: "3"})

					conn := dialProxy(proxyServer)
					req := test_util.NewRequest("GET", "fully-broken-app", "/", nil)

					started := time.Now()
					conn.WriteRequest(req)

					resp, err := http.ReadResponse(conn.Reader, &http.Request{})
					Expect(err).NotTo(HaveOccurred())

					Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
					Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

					Eventually(func() (int64, error) {
						fi, err := f.Stat()
						if err != nil {
							return 0, err
						}
						return fi.Size(), nil
					}, "10s").ShouldNot(BeZero())

					logBytes, err := os.ReadFile(f.Name())
					Expect(err).NotTo(HaveOccurred())
					logStr := string(logBytes)
					Expect(strings.HasPrefix(logStr, "fully-broken-app - [")).To(BeTrue())
					Expect(logStr).To(ContainSubstring("response_time:"))
					Expect(logStr).To(ContainSubstring("gorouter_time:"))
					Expect(logStr).To(ContainSubstring("failed_attempts:3"))
					Expect(logStr).To(ContainSubstring("failed_attempts_time:0.3"))
					Expect(logStr).To(ContainSubstring("response_time:0.3"))
					Expect(logStr).To(ContainSubstring("gorouter_time:0"))
					Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
					Expect(logStr).To(ContainSubstring(`dns_time:"-"`))
					Expect(logStr).To(ContainSubstring(`dial_time:"-"`))
					Expect(logStr).To(ContainSubstring(`tls_time:"-"`))
				})
				Context("in websocket requests", func() {
					BeforeEach(func() {
						conf.WebsocketDialTimeout = 100 * time.Millisecond
					})
					It("shows slowness in failed_attempts_time and backend_time, not gorouter_time", func() {

						// Register some broken endpoints to cause retries
						test_util.RegisterAddr(r, "partially-broken-ws-app", "10.255.255.1:1234", test_util.RegisterConfig{InstanceIndex: "1"})
						test_util.RegisterAddr(r, "partially-broken-ws-app", "10.255.255.1:1235", test_util.RegisterConfig{InstanceIndex: "2"})

						ln := test_util.RegisterWSHandler(r, "partially-broken-ws-app", func(conn *websocket.Conn) {
							msgBuf := make([]byte, 100)
							n, err := conn.Read(msgBuf)
							Expect(err).NotTo(HaveOccurred())
							Expect(string(msgBuf[:n])).To(Equal("HELLO WEBSOCKET"))

							time.Sleep(100 * time.Millisecond)

							_, _ = conn.Write([]byte("WEBSOCKET OK"))
							conn.Close()
						}, test_util.RegisterConfig{InstanceIndex: "3"})
						defer ln.Close()

						Eventually(func(g Gomega) {
							conn := dialProxy(proxyServer)
							wsConn, err := wsClient(conn, "ws://partially-broken-ws-app")
							g.Expect(err).NotTo(HaveOccurred())

							_, err = wsConn.Write([]byte("HELLO WEBSOCKET"))
							g.Expect(err).NotTo(HaveOccurred())

							msgBuf := make([]byte, 100)
							n, err := wsConn.Read(msgBuf)

							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(string(msgBuf[:n])).To(Equal("WEBSOCKET OK"))

							g.Eventually(func() (int64, error) {
								fi, err := f.Stat()
								if err != nil {
									return 0, err
								}
								return fi.Size(), nil
							}).ShouldNot(BeZero())

							//make sure the record includes all the data
							//since the building of the log record happens throughout the life of the request
							logBytes, err := os.ReadFile(f.Name())
							g.Expect(err).NotTo(HaveOccurred())
							logStr := string(logBytes)
							g.Expect(strings.HasPrefix(logStr, "partially-broken-ws-app - [")).To(BeTrue())
							g.Expect(logStr).To(ContainSubstring(`"GET / HTTP/1.1" 101`))
							g.Expect(logStr).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
							g.Expect(logStr).To(ContainSubstring("backend_time:0.1"))         // 0.1 seconds delay from slow backend app
							g.Expect(logStr).To(ContainSubstring("failed_attempts_time:0.2")) // plus 0.2 seconds from dial attempts
							g.Expect(logStr).To(ContainSubstring("response_time:0.3"))        // makes 0.3 seconds total response time
							g.Expect(logStr).To(ContainSubstring("gorouter_time:0.0"))
							g.Expect(logStr).To(ContainSubstring("failed_attempts:2"))
							g.Expect(logStr).To(ContainSubstring(`app_index:"3"`))
						}, "20s").Should(Succeed())
					})
					It("shows no backend_time or other attempt details if all endpoints are broken", func() {

						// Register some broken endpoints to cause retries
						test_util.RegisterAddr(r, "fully-broken-ws-app", "10.255.255.1:1234", test_util.RegisterConfig{InstanceIndex: "1"})
						test_util.RegisterAddr(r, "fully-broken-ws-app", "10.255.255.1:1235", test_util.RegisterConfig{InstanceIndex: "2"})
						test_util.RegisterAddr(r, "fully-broken-ws-app", "10.255.255.1:1236", test_util.RegisterConfig{InstanceIndex: "3"})

						conn := dialProxy(proxyServer)
						wsConn, err := wsClient(conn, "ws://fully-broken-ws-app")
						Expect(err).To(HaveOccurred())
						Expect(wsConn).To(BeNil())

						Eventually(func() (int64, error) {
							fi, err := f.Stat()
							if err != nil {
								return 0, err
							}
							return fi.Size(), nil
						}, "10s").ShouldNot(BeZero())

						logBytes, err := os.ReadFile(f.Name())
						Expect(err).NotTo(HaveOccurred())
						logStr := string(logBytes)
						Expect(strings.HasPrefix(logStr, "fully-broken-ws-app - [")).To(BeTrue())
						Expect(logStr).To(ContainSubstring("response_time:"))
						Expect(logStr).To(ContainSubstring("gorouter_time:"))
						Expect(logStr).To(ContainSubstring("failed_attempts:3"))
						Expect(logStr).To(ContainSubstring("failed_attempts_time:0.3"))
						Expect(logStr).To(ContainSubstring("response_time:0.3"))
						Expect(logStr).To(ContainSubstring("gorouter_time:0.0"))
						Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
						Expect(logStr).To(ContainSubstring(`dns_time:"-"`))
						Expect(logStr).To(ContainSubstring(`dial_time:"-"`))
						Expect(logStr).To(ContainSubstring(`tls_time:"-"`))
					})
				})
			})
		})

		Context("lookup errors when attempt details logging is enabled", func() {
			BeforeEach(func() {
				conf.Logging.ExtraAccessLogFields = []string{"failed_attempts", "failed_attempts_time", "dns_time", "dial_time", "tls_time", "backend_time"}
			})

			It("logs no backend_time on missing app route", func() {
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "does-not-exist", "/", nil)

				started := time.Now()
				conn.WriteRequest(req)

				resp, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}, "5s").ShouldNot(BeZero())

				logBytes, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				logStr := string(logBytes)
				Expect(strings.HasPrefix(logStr, "does-not-exist - [")).To(BeTrue())
				Expect(logStr).To(ContainSubstring("response_time:"))
				Expect(logStr).To(ContainSubstring("gorouter_time:"))
				Expect(logStr).To(ContainSubstring("response_time:0"))
				Expect(logStr).To(ContainSubstring("gorouter_time:0"))
				Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
			})
			It("logs no backend_time on invalid X-CF-App-Instance header", func() {
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "invalid-app-instance-header", "/", nil)
				req.Header.Set("X-CF-App-Instance", "invalid-instance")
				started := time.Now()
				conn.WriteRequest(req)

				resp, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}, "5s").ShouldNot(BeZero())

				logBytes, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				logStr := string(logBytes)
				Expect(strings.HasPrefix(logStr, "invalid-app-instance-header - [")).To(BeTrue())
				Expect(logStr).To(ContainSubstring("response_time:"))
				Expect(logStr).To(ContainSubstring("gorouter_time:"))
				Expect(logStr).To(ContainSubstring("response_time:0"))
				Expect(logStr).To(ContainSubstring("gorouter_time:0"))
				Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
			})
			It("logs no backend_time on empty pools (404 status)", func() {
				test_util.RegisterAddr(r, "empty-pool-app", "10.255.255.1:1234", test_util.RegisterConfig{StaleThreshold: 1})

				// Wait 1s for the endpoint to become stale
				time.Sleep(1 * time.Second)

				pool := r.Lookup("empty-pool-app")
				pruned := pool.PruneEndpoints()

				Expect(pruned).NotTo(BeEmpty())
				Expect(pool.IsEmpty()).To(BeTrue())

				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "empty-pool-app", "/", nil)
				started := time.Now()
				conn.WriteRequest(req)

				resp, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}, "5s").ShouldNot(BeZero())

				logBytes, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				logStr := string(logBytes)
				Expect(strings.HasPrefix(logStr, "empty-pool-app - [")).To(BeTrue())
				Expect(logStr).To(ContainSubstring("response_time:"))
				Expect(logStr).To(ContainSubstring("gorouter_time:"))
				Expect(logStr).To(ContainSubstring("response_time:0"))
				Expect(logStr).To(ContainSubstring("gorouter_time:0"))
				Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
			})
			Context("when EmptyPoolResponseCode503 is enabled", func() {
				BeforeEach(func() {
					conf.EmptyPoolResponseCode503 = true
					conf.EmptyPoolTimeout = 30 * time.Second
				})
				It("logs no backend_time on empty pools (503 status)", func() {
					test_util.RegisterAddr(r, "empty-pool-app", "10.255.255.1:1234", test_util.RegisterConfig{StaleThreshold: 1})

					// Wait 1s for the endpoint to become stale
					time.Sleep(1 * time.Second)

					pool := r.Lookup("empty-pool-app")
					pruned := pool.PruneEndpoints()

					Expect(pruned).NotTo(BeEmpty())
					Expect(pool.IsEmpty()).To(BeTrue())

					conn := dialProxy(proxyServer)

					req := test_util.NewRequest("GET", "empty-pool-app", "/", nil)
					started := time.Now()
					conn.WriteRequest(req)

					resp, err := http.ReadResponse(conn.Reader, &http.Request{})
					Expect(err).NotTo(HaveOccurred())

					Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
					Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))

					Eventually(func() (int64, error) {
						fi, err := f.Stat()
						if err != nil {
							return 0, err
						}
						return fi.Size(), nil
					}, "5s").ShouldNot(BeZero())

					logBytes, err := os.ReadFile(f.Name())
					Expect(err).NotTo(HaveOccurred())
					logStr := string(logBytes)
					Expect(strings.HasPrefix(logStr, "empty-pool-app - [")).To(BeTrue())
					Expect(logStr).To(ContainSubstring("response_time:"))
					Expect(logStr).To(ContainSubstring("gorouter_time:"))
					Expect(logStr).To(ContainSubstring("response_time:0"))
					Expect(logStr).To(ContainSubstring("gorouter_time:0"))
					Expect(logStr).To(ContainSubstring(`backend_time:"-"`))
				})
			})
		})

		Context("when an HTTP 100 continue response is sent first", func() {
			It("logs the data for the final response", func() {
				ln := test_util.RegisterConnHandler(r, "test", func(conn *test_util.HttpConn) {
					req, body := conn.ReadRequest()
					Expect(req.Method).To(Equal("POST"))
					Expect(req.URL.Path).To(Equal("/"))
					Expect(req.ProtoMajor).To(Equal(1))
					Expect(req.ProtoMinor).To(Equal(1))

					Expect(body).To(Equal("ABCD"))

					expectRsp := test_util.NewResponse(100)
					conn.WriteResponse(expectRsp)

					rsp := test_util.NewResponse(200)
					rsp.Body = io.NopCloser(strings.NewReader("valid-but-unimportant-response-data"))
					conn.WriteResponse(rsp)

					conn.Close()
				}, test_util.RegisterConfig{InstanceId: "123", AppId: "456"})
				defer ln.Close()

				conn := dialProxy(proxyServer)

				body := &bytes.Buffer{}
				body.WriteString("ABCD")
				req := test_util.NewRequest("POST", "test", "/", body)
				conn.WriteRequest(req)

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusContinue))

				resp, _ = conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}).ShouldNot(BeZero())

				//make sure the record includes all the data
				//since the building of the log record happens throughout the life of the request
				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				Expect(string(b)).To(HavePrefix("test - ["))
				Expect(string(b)).To(ContainSubstring(`"POST / HTTP/1.1" 200 4 35 "-"`))
				Expect(string(b)).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
				Expect(string(b)).To(ContainSubstring(`response_time:`))
				Expect(string(b)).To(ContainSubstring(`app_id:"456"`))
				Expect(string(b)).To(ContainSubstring(`app_index:"2"`))
				Expect(string(b)).To(HaveSuffix("\n"))
			})

			It("returns the correct status code to the client", func() {
				ln := test_util.RegisterConnHandler(r, "statusTest", func(conn *test_util.HttpConn) {
					req, body := conn.ReadRequest()
					Expect(req.Method).To(Equal("POST"))
					Expect(req.URL.Path).To(Equal("/"))
					Expect(req.ProtoMajor).To(Equal(1))
					Expect(req.ProtoMinor).To(Equal(1))

					Expect(body).To(Equal("ABCD"))

					expectRsp := test_util.NewResponse(100)
					conn.WriteResponse(expectRsp)

					rsp := test_util.NewResponse(201)
					rsp.Body = io.NopCloser(strings.NewReader("valid-but-unimportant-response-data"))
					conn.WriteResponse(rsp)

					conn.Close()
				}, test_util.RegisterConfig{InstanceId: "123", AppId: "456"})
				defer ln.Close()

				conn := dialProxy(proxyServer)

				body := &bytes.Buffer{}
				body.WriteString("ABCD")
				req := test_util.NewRequest("POST", "statusTest", "/", body)
				conn.WriteRequest(req)

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusContinue))

				resp, _ = conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}).ShouldNot(BeZero())

				//make sure the record includes all the data
				//since the building of the log record happens throughout the life of the request
				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				Expect(string(b)).To(HavePrefix("statusTest - ["))
				Expect(string(b)).To(ContainSubstring(`"POST / HTTP/1.1" 201 4 35 "-"`))
				Expect(string(b)).To(ContainSubstring(`x_forwarded_for:"127.0.0.1" x_forwarded_proto:"http" vcap_request_id:`))
				Expect(string(b)).To(ContainSubstring(`response_time:`))
				Expect(string(b)).To(ContainSubstring(`app_id:"456"`))
				Expect(string(b)).To(ContainSubstring(`app_index:"2"`))
				Expect(string(b)).To(HaveSuffix("\n"))

			})
		})

		It("logs a request when X-Forwarded-Proto and X-Forwarded-For are provided", func() {
			ln := test_util.RegisterConnHandler(r, "test", func(conn *test_util.HttpConn) {
				conn.ReadRequest()
				conn.WriteResponse(test_util.NewResponse(http.StatusOK))
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("POST", "test", "/", nil)
			req.Header.Add("X-Forwarded-For", "1.2.3.4")
			req.Header.Add("X-Forwarded-Proto", "https")
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Eventually(func() (int64, error) {
				fi, err := f.Stat()
				if err != nil {
					return 0, err
				}
				return fi.Size(), nil
			}).ShouldNot(BeZero())

			//make sure the record includes all the data
			//since the building of the log record happens throughout the life of the request
			b, err := os.ReadFile(f.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.HasPrefix(string(b), "test - [")).To(BeTrue())
			Expect(string(b)).To(ContainSubstring(`"POST / HTTP/1.1" 200 0 0 "-"`))
			Expect(string(b)).To(ContainSubstring(`x_forwarded_for:"1.2.3.4, 127.0.0.1" x_forwarded_proto:"https" vcap_request_id:`))
			Expect(string(b)).To(ContainSubstring(`response_time:`))
			Expect(b[len(b)-1]).To(Equal(byte('\n')))
		})

		It("logs a request when it exits early", func() {
			conn := dialProxy(proxyServer)

			body := &bytes.Buffer{}
			body.WriteString("ABCD")
			req := test_util.NewRequest("POST", "test", "/", io.NopCloser(body))
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

			Eventually(func() (int64, error) {
				fi, err := f.Stat()
				if err != nil {
					return 0, err
				}
				return fi.Size(), nil
			}).ShouldNot(BeZero())

			b, err := os.ReadFile(f.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(MatchRegexp("^test.*\n"))
		})

		Context("when the request has X-CF-APP-INSTANCE", func() {
			var (
				conn  *test_util.HttpConn
				uuid1 *uuid.UUID
				uuid2 *uuid.UUID
				ln    net.Listener
				ln2   net.Listener
			)

			JustBeforeEach(func() {
				uuid1, _ = uuid.NewV4()
				uuid2, _ = uuid.NewV4()

				ln = test_util.RegisterConnHandler(r, "app."+test_util.LocalhostDNS, func(conn *test_util.HttpConn) {
					Fail("App should not have received request")
				}, test_util.RegisterConfig{AppId: uuid1.String()})
				defer ln.Close()

				ln2 = test_util.RegisterConnHandler(r, "app."+test_util.LocalhostDNS, func(conn *test_util.HttpConn) {
					req, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())

					Expect(req.Header.Get(router_http.CfAppInstance)).To(BeEmpty())

					resp := test_util.NewResponse(http.StatusOK)
					resp.Body = io.NopCloser(strings.NewReader("Hellow World: App2"))
					conn.WriteResponse(resp)

					conn.Close()

				}, test_util.RegisterConfig{AppId: uuid2.String(), InstanceIndex: "0"})
				conn = dialProxy(proxyServer)
			})

			It("lookups the route to that specific app index and id", func() {
				defer ln2.Close()
				defer ln.Close()
				req := test_util.NewRequest("GET", "app."+test_util.LocalhostDNS, "/", nil)
				req.Header.Set(router_http.CfAppInstance, uuid2.String()+":0")

				Consistently(func() string {
					conn.WriteRequest(req)

					_, b := conn.ReadResponse()
					return b
				}).Should(Equal("Hellow World: App2"))
			})

			It("returns a 400 if it cannot find the specified instance", func() {
				req := test_util.NewRequest("GET", "app."+test_util.LocalhostDNS, "/", nil)
				req.Header.Set("X-CF-APP-INSTANCE", uuid1.String()+":1")
				conn.WriteRequest(req)

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("X-Cf-RouterError")).To(Equal("unknown_route"))
			})
		})

		Context("with EnableZipkin set to true", func() {
			BeforeEach(func() {
				conf.Tracing.EnableZipkin = true
			})

			It("x_b3_traceid does show up in the access log", func() {
				done := make(chan string)
				ln := test_util.RegisterConnHandler(r, "app", func(conn *test_util.HttpConn) {
					req, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())

					resp := test_util.NewResponse(http.StatusOK)
					conn.WriteResponse(resp)
					conn.Close()

					done <- req.Header.Get(b3.TraceID)
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)
				req := test_util.NewRequest("GET", "app", "/", nil)
				conn.WriteRequest(req)

				var answer string
				Eventually(done).Should(Receive(&answer))
				Expect(answer).ToNot(BeEmpty())

				conn.ReadResponse()

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}).ShouldNot(BeZero())

				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())

				Expect(string(b)).To(ContainSubstring(fmt.Sprintf(`x_b3_traceid:"%s"`, answer)))
			})
		})
	})

	Describe("User-Agent Healthcheck", func() {
		It("responds to load balancer check", func() {
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "", "/", nil)
			req.Header.Set("User-Agent", "HTTP-Monitor/1.1")
			conn.WriteRequest(req)

			resp, body := conn.ReadResponse()
			Expect(resp.Header.Get("Cache-Control")).To(Equal("private, max-age=0"))
			Expect(resp.Header.Get("Expires")).To(Equal("0"))
			Expect(resp.Status).To(Equal("200 OK"))
			Expect(body).To(Equal("ok\n"))
		})

		It("responds with failure to load balancer check if healthStatus is unhealthy", func() {
			healthStatus.SetHealth(health.Degraded)
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "", "/", nil)
			req.Header.Set("User-Agent", "HTTP-Monitor/1.1")
			conn.WriteRequest(req)

			resp, body := conn.ReadResponse()
			Expect(resp.Header.Get("Cache-Control")).To(Equal("private, max-age=0"))
			Expect(resp.Header.Get("Expires")).To(Equal("0"))
			Expect(resp.Status).NotTo(Equal("200 OK"))
			Expect(body).NotTo(Equal("ok\n"))
		})
	})

	Describe("Error Responses", func() {
		It("responds to unknown host with 404", func() {
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "unknown", "/", nil)
			conn.WriteRequest(req)

			resp, body := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			Expect(resp.Header.Get("X-Cf-RouterError")).To(Equal("unknown_route"))
			Expect(body).To(Equal("404 Not Found: Requested route ('unknown') does not exist.\n"))
		})

		It("responds to host with malicious script with 400", func() {
			conn, err := net.Dial("tcp", proxyServer.Addr().String())
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			rawReq := "GET / HTTP/1.1\nHost: <html><header><script>alert(document.cookie);</script></header><body/></html>\n\n\n"

			conn.Write([]byte(rawReq))

			resp, err := io.ReadAll(conn)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(resp)).To(ContainSubstring("HTTP/1.1 400 Bad Request"))               // status header
			Expect(string(resp)).To(ContainSubstring("400 Bad Request: malformed Host header")) // body
		})

		It("responds with 404 for a not found host name with only valid characters", func() {
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "abcdefghijklmnopqrstuvwxyz.0123456789-ABCDEFGHIJKLMNOPQRSTUVW.XYZ", "/", nil)
			conn.WriteRequest(req)

			resp, body := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			Expect(resp.Header.Get("X-Cf-RouterError")).To(Equal("unknown_route"))
			Expect(body).To(Equal("404 Not Found: Requested route ('abcdefghijklmnopqrstuvwxyz.0123456789-ABCDEFGHIJKLMNOPQRSTUVW.XYZ') does not exist.\n"))
		})

		It("responds to misbehaving host with 502", func() {
			ln := test_util.RegisterConnHandler(r, "enfant-terrible", func(conn *test_util.HttpConn) {
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "enfant-terrible", "/", nil)
			conn.WriteRequest(req)

			resp, body := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(resp.Header.Get("X-Cf-RouterError")).To(ContainSubstring("endpoint_failure"))
			Expect(body).To(Equal("502 Bad Gateway: Registered endpoint failed to handle the request.\n"))
		})

		Context("when the endpoint is nil", func() {
			removeAllEndpoints := func(pool *route.EndpointPool) {
				endpoints := make([]*route.Endpoint, 0)
				pool.Each(func(e *route.Endpoint) {
					endpoints = append(endpoints, e)
				})
				for _, e := range endpoints {
					pool.Remove(e)
				}
			}

			nilEndpointsTest := func(expectedStatusCode int) {
				ln := test_util.RegisterConnHandler(r, "nil-endpoint", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET / HTTP/1.1")
					resp := test_util.NewResponse(http.StatusOK)
					conn.WriteResponse(resp)
					conn.Close()
				})
				defer ln.Close()

				removeAllEndpoints(r.Lookup(route.Uri("nil-endpoint")))
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "nil-endpoint", "/", nil)
				conn.WriteRequest(req)

				b := make([]byte, 0)
				buf := bytes.NewBuffer(b)
				log.SetOutput(buf)
				res, _ := conn.ReadResponse()
				log.SetOutput(os.Stderr)
				Expect(buf).NotTo(ContainSubstring("multiple response.WriteHeader calls"))
				Expect(res.StatusCode).To(Equal(expectedStatusCode))
			}

			Context("when emptyPoolResponseCode503 is true", func() {
				BeforeEach(func() {
					conf.EmptyPoolResponseCode503 = true
				})

				It("responds with a 503 ServiceUnavailable", func() {
					nilEndpointsTest(http.StatusServiceUnavailable)
				})
			})

			Context("when emptyPoolResponseCode503 is false", func() {
				BeforeEach(func() {
					conf.EmptyPoolResponseCode503 = false
				})

				It("responds with a 404 NotFound", func() {
					nilEndpointsTest(http.StatusNotFound)
				})
			})
		})

		Context("when the round trip errors and original client has disconnected", func() {
			It("response code is always 499", func() {
				ln := test_util.RegisterConnHandler(r, "post-some-data", func(conn *test_util.HttpConn) {
					req, err := http.ReadRequest(conn.Reader)
					if err != nil {
						fmt.Println(err)
						return
					}
					defer req.Body.Close()
					Expect(req.Method).To(Equal("POST"))
					Expect(req.URL.Path).To(Equal("/"))
					Expect(req.ProtoMajor).To(Equal(1))
					Expect(req.ProtoMinor).To(Equal(1))
					io.ReadAll(req.Body)
					rsp := test_util.NewResponse(200)
					conn.WriteResponse(rsp)
					conn.Close()
				}, test_util.RegisterConfig{InstanceId: "499", AppId: "502"})
				defer ln.Close()

				payloadSize := 2 << 24
				payload := strings.Repeat("a", payloadSize)
				sendrequest := func(wg *sync.WaitGroup) {
					defer wg.Done()
					req := test_util.NewRequest("POST", "post-some-data", "/", bytes.NewReader([]byte(payload)))
					tcpaddr, err := net.ResolveTCPAddr("tcp", proxyServer.Addr().String())
					Expect(err).ToNot(HaveOccurred())
					conn, err := net.DialTCP("tcp", nil, tcpaddr)
					Expect(err).ToNot(HaveOccurred())
					conn.SetLinger(0)
					conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
					writer := bufio.NewWriter(conn)
					go func(req *http.Request, writer *bufio.Writer) {
						defer GinkgoRecover()
						err = req.Write(writer)
						Expect(err).To(HaveOccurred())
						writer.Flush()
					}(req, writer)
					time.Sleep(10 * time.Millisecond) // give time for the data to start transmitting

					// need to shutdown the writer first or conn.Close will block until the large payload finishes sending.
					// Another way to do this is to get syscall.rawconn and use control to syscall.SetNonblock on the
					// connections file descriptor
					conn.CloseWrite()
					conn.Close()
				}

				var wg sync.WaitGroup
				for i := 0; i < 4; i++ {
					wg.Add(1)
					go sendrequest(&wg)
				}
				wg.Wait()

				Eventually(func() (int64, error) {
					fi, err := f.Stat()
					if err != nil {
						return 0, err
					}
					return fi.Size(), nil
				}).ShouldNot(BeZero())

				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())
				Expect(string(b)).To(ContainSubstring("HTTP/1.1\" 499"))
				Expect(string(b)).ToNot(ContainSubstring("HTTP/1.1\" 502"))
				Expect(string(b)).ToNot(ContainSubstring("HTTP/1.1\" 503"))
			})
		})
	})

	Describe("WebSocket Connections", func() {
		Context("when the request is mapped to route service", func() {

			It("responds with 503", func() {
				done := make(chan bool)

				ln := test_util.RegisterConnHandler(r, "ws", func(conn *test_util.HttpConn) {
					req, err := http.ReadRequest(conn.Reader)
					Expect(err).NotTo(HaveOccurred())

					done <- req.Header.Get("Upgrade") == "Websocket" &&
						req.Header.Get("Connection") == "Upgrade"

					resp := test_util.NewResponse(http.StatusSwitchingProtocols)
					resp.Header.Set("Upgrade", "Websocket")
					resp.Header.Set("Connection", "Upgrade")

					conn.WriteResponse(resp)

					conn.CheckLine("hello from client")
					conn.WriteLine("hello from server")
					conn.Close()
				})
				defer ln.Close()

				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "ws", "/chat", nil)
				req.Header.Set("Upgrade", "Websocket")
				req.Header.Set("Connection", "Upgrade")

				conn.WriteRequest(req)

				var answer bool
				Eventually(done).Should(Receive(&answer))
				Expect(answer).To(BeTrue())

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))
				Expect(resp.Header.Get("Upgrade")).To(Equal("Websocket"))
				Expect(resp.Header.Get("Connection")).To(Equal("Upgrade"))

				conn.WriteLine("hello from client")
				conn.CheckLine("hello from server")

				conn.Close()
			})
		})

		It("upgrades for a WebSocket request", func() {
			done := make(chan bool)

			ln := test_util.RegisterConnHandler(r, "ws", func(conn *test_util.HttpConn) {
				req, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				done <- req.Header.Get("Upgrade") == "Websocket" &&
					req.Header.Get("Connection") == "Upgrade"

				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				conn.WriteResponse(resp)

				conn.CheckLine("hello from client")
				conn.WriteLine("hello from server")
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "ws", "/chat", nil)
			req.Header.Set("Upgrade", "Websocket")
			req.Header.Set("Connection", "Upgrade")

			conn.WriteRequest(req)

			var answer bool
			Eventually(done).Should(Receive(&answer))
			Expect(answer).To(BeTrue())

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))
			Expect(resp.Header.Get("Upgrade")).To(Equal("Websocket"))
			Expect(resp.Header.Get("Connection")).To(Equal("Upgrade"))

			conn.WriteLine("hello from client")
			conn.CheckLine("hello from server")

			conn.Close()
		})

		It("upgrades for a WebSocket request with comma-separated Connection header", func() {
			done := make(chan bool)

			ln := test_util.RegisterConnHandler(r, "ws-cs-header", func(conn *test_util.HttpConn) {
				req, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				// RFC 7230, section 6.1: Remove headers listed in the "Connection" header.
				// Only "Upgrade" will be added again by httputil.ReverseProxy
				done <- req.Header.Get("Upgrade") == "Websocket" &&
					req.Header.Get("Connection") == "Upgrade"

				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				conn.WriteResponse(resp)

				conn.CheckLine("hello from client")
				conn.WriteLine("hello from server")
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "ws-cs-header", "/chat", nil)
			req.Header.Add("Upgrade", "Websocket")
			req.Header.Add("Connection", "keep-alive, Upgrade")

			conn.WriteRequest(req)

			var answer bool
			Eventually(done).Should(Receive(&answer))
			Expect(answer).To(BeTrue())

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))

			Expect(resp.Header.Get("Upgrade")).To(Equal("Websocket"))
			Expect(resp.Header.Get("Connection")).To(Equal("Upgrade"))

			conn.WriteLine("hello from client")
			conn.CheckLine("hello from server")

			conn.Close()
		})

		It("upgrades for a WebSocket request with multiple Connection headers", func() {
			done := make(chan bool)

			ln := test_util.RegisterConnHandler(r, "ws-cs-header", func(conn *test_util.HttpConn) {
				req, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				// RFC 7230, section 6.1: Remove headers listed in the "Connection" header.
				// Only "Upgrade" will be added again by httputil.ReverseProxy
				done <- req.Header.Get("Upgrade") == "Websocket" &&
					req.Header.Get("Connection") == "Upgrade"

				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				conn.WriteResponse(resp)

				conn.CheckLine("hello from client")
				conn.WriteLine("hello from server")
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "ws-cs-header", "/chat", nil)
			req.Header.Add("Upgrade", "Websocket")
			req.Header.Add("Connection", "keep-alive")
			req.Header.Add("Connection", "Upgrade")

			conn.WriteRequest(req)

			var answer bool
			Eventually(done).Should(Receive(&answer))
			Expect(answer).To(BeTrue())

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))

			Expect(resp.Header.Get("Upgrade")).To(Equal("Websocket"))
			Expect(resp.Header.Get("Connection")).To(Equal("Upgrade"))

			conn.WriteLine("hello from client")
			conn.CheckLine("hello from server")

			conn.Close()
		})

		It("logs the response time and status code 101 in the access logs", func() {
			done := make(chan bool)
			ln := test_util.RegisterConnHandler(r, "ws", func(conn *test_util.HttpConn) {
				req, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				done <- req.Header.Get("Upgrade") == "Websocket" &&
					req.Header.Get("Connection") == "Upgrade"

				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				conn.WriteResponse(resp)

				conn.CheckLine("hello from client")
				conn.WriteLine("hello from server")
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "ws", "/chat", nil)
			req.Header.Set("Upgrade", "Websocket")
			req.Header.Set("Connection", "Upgrade")

			conn.WriteRequest(req)

			var answer bool
			Eventually(done).Should(Receive(&answer))
			Expect(answer).To(BeTrue())

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))
			Expect(resp.Header.Get("Upgrade")).To(Equal("Websocket"))
			Expect(resp.Header.Get("Connection")).To(Equal("Upgrade"))

			conn.WriteLine("hello from client")
			conn.CheckLine("hello from server")

			Eventually(func() (int64, error) {
				fi, err := f.Stat()
				if err != nil {
					return 0, err
				}
				return fi.Size(), nil
			}).ShouldNot(BeZero())

			b, err := os.ReadFile(f.Name())
			Expect(err).NotTo(HaveOccurred())

			Expect(string(b)).To(ContainSubstring(`response_time:`))
			Expect(string(b)).To(ContainSubstring("HTTP/1.1\" 101"))
			responseTime := parseResponseTimeFromLog(string(b))
			Expect(responseTime).To(BeNumerically(">", 0))

			conn.Close()
		})

		It("emits a xxx metric", func() {
			ln := test_util.RegisterConnHandler(r, "ws-cs-header", func(conn *test_util.HttpConn) {
				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				req, _ := conn.ReadRequest()
				Expect(req.Header.Values("Connection")).To(ContainElement("Upgrade"))
				Expect(req.Header.Get("Upgrade")).To(Equal("Websocket"))
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			connectClient := func() {
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "ws-cs-header", "/chat", nil)
				req.Header.Add("Upgrade", "Websocket")
				req.Header.Add("Connection", "keep-alive")
				req.Header.Add("Connection", "Upgrade")

				conn.WriteRequest(req)
				conn.ReadResponse()
			}
			// 1st client connected
			connectClient()
			// 2nd client connected
			connectClient()

			Eventually(fakeReporter.CaptureWebSocketUpdateCallCount).Should(Equal(2))
		})

		It("does emit a latency metric", func() {
			var wg sync.WaitGroup
			ln := test_util.RegisterConnHandler(r, "ws-cs-header", func(conn *test_util.HttpConn) {
				defer conn.Close()
				defer wg.Done()
				resp := test_util.NewResponse(http.StatusSwitchingProtocols)
				resp.Header.Set("Upgrade", "Websocket")
				resp.Header.Set("Connection", "Upgrade")

				req, _ := conn.ReadRequest()
				Expect(req.Header.Values("Connection")).To(ContainElement("Upgrade"))
				Expect(req.Header.Get("Upgrade")).To(Equal("Websocket"))

				conn.WriteResponse(resp)

				for {
					_, err := conn.Write([]byte("Hello"))
					if err != nil {
						return
					}
				}
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "ws-cs-header", "/chat", nil)
			req.Header.Add("Upgrade", "Websocket")
			req.Header.Add("Connection", "keep-alive")
			req.Header.Add("Connection", "Upgrade")

			wg.Add(1)
			conn.WriteRequest(req)
			resp, err := http.ReadResponse(conn.Reader, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusSwitchingProtocols))
			buf := make([]byte, 5)
			_, err = conn.Read(buf)
			Expect(err).ToNot(HaveOccurred())
			conn.Close()
			wg.Wait()

			Consistently(fakeReporter.CaptureRoutingResponseLatencyCallCount, 1).Should(Equal(1))
		})

		Context("when the connection to the backend fails", func() {
			It("emits a failure metric and logs a 502 in the access logs", func() {
				test_util.RegisterAddr(r, "ws", "192.0.2.1:1234", test_util.RegisterConfig{
					InstanceIndex: "2",
					AppId:         "abc",
				})

				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "ws", "/chat", nil)
				req.Header.Set("Upgrade", "Websocket")
				req.Header.Set("Connection", "Upgrade")

				conn.WriteRequest(req)

				res, err := http.ReadResponse(conn.Reader, &http.Request{})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusBadGateway))

				Eventually(func() (int64, error) {
					fi, fErr := f.Stat()
					if fErr != nil {
						return 0, fErr
					}
					return fi.Size(), nil
				}).ShouldNot(BeZero())

				b, err := os.ReadFile(f.Name())
				Expect(err).NotTo(HaveOccurred())

				Expect(string(b)).To(ContainSubstring(`response_time:`))
				Expect(string(b)).To(ContainSubstring("HTTP/1.1\" 502"))
				responseTime := parseResponseTimeFromLog(string(b))
				Expect(responseTime).To(BeNumerically(">", 0))

				Expect(fakeReporter.CaptureWebSocketUpdateCallCount()).To(Equal(0))
				Expect(fakeReporter.CaptureWebSocketFailureCallCount()).To(Equal(1))
				conn.Close()
			})
		})
	})

	Describe("Metrics", func() {
		It("captures the routing response", func() {
			ln := test_util.RegisterConnHandler(r, "reporter-test", func(conn *test_util.HttpConn) {
				_, err := http.ReadRequest(conn.Reader)
				Expect(err).NotTo(HaveOccurred())

				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			})
			defer ln.Close()

			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "reporter-test", "/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(fakeReporter.CaptureBadGatewayCallCount()).To(Equal(0))

			Expect(fakeReporter.CaptureRoutingResponseCallCount()).To(Equal(1))
			capturedRespCode := fakeReporter.CaptureRoutingResponseArgsForCall(0)
			Expect(capturedRespCode).To(Equal(http.StatusOK))

			Expect(fakeReporter.CaptureRoutingResponseLatencyCallCount()).To(Equal(1))
			capturedEndpoint, capturedRespCode, startTime, latency := fakeReporter.CaptureRoutingResponseLatencyArgsForCall(0)
			Expect(capturedEndpoint).ToNot(BeNil())
			Expect(capturedEndpoint.ApplicationId).To(Equal(""))
			Expect(capturedEndpoint.PrivateInstanceId).To(Equal(""))
			Expect(capturedEndpoint.PrivateInstanceIndex).To(Equal("2"))
			Expect(capturedRespCode).To(Equal(http.StatusOK))
			Expect(startTime).To(BeTemporally("~", time.Now(), 100*time.Millisecond))
			Expect(latency).To(BeNumerically(">", 0))

			Expect(fakeReporter.CaptureRoutingRequestCallCount()).To(Equal(1))
			Expect(fakeReporter.CaptureRoutingRequestArgsForCall(0)).To(Equal(capturedEndpoint))
		})

		It("emits HTTP startstop events", func() {
			var vcapHeader string
			ln := test_util.RegisterConnHandler(r, "app", func(conn *test_util.HttpConn) {
				req, _ := conn.ReadRequest()
				vcapHeader = req.Header.Get(handlers.VcapRequestIdHeader)
				resp := test_util.NewResponse(http.StatusOK)
				conn.WriteResponse(resp)
				conn.Close()
			}, test_util.RegisterConfig{InstanceId: "fake-instance-id"})
			defer ln.Close()
			conn := dialProxy(proxyServer)

			req := test_util.NewRequest("GET", "app", "/", nil)
			conn.WriteRequest(req)

			resp, _ := conn.ReadResponse()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			findStartStopEvent := func() *events.HttpStartStop {
				for _, ev := range fakeEmitter.GetEvents() {
					startStopEvent, ok := ev.(*events.HttpStartStop)
					if ok {
						return startStopEvent
					}
				}
				return nil
			}

			Eventually(findStartStopEvent).ShouldNot(BeNil())
			u2, err := uuid.ParseHex(vcapHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(findStartStopEvent().RequestId).To(Equal(factories.NewUUID(u2)))
		})

		Context("when http prometheus metrics are turned on", func() {
			BeforeEach(func() {
				conf.PerAppPrometheusHttpMetricsReporting = true
			})
			It("records http prometheus metrics", func() {
				ln := test_util.RegisterConnHandler(r, "app", func(conn *test_util.HttpConn) {
					conn.ReadRequest()
					resp := test_util.NewResponse(http.StatusOK)
					conn.WriteResponse(resp)
					conn.Close()
				}, test_util.RegisterConfig{InstanceId: "fake-instance-id"})
				defer ln.Close()
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "app", "/", nil)
				conn.WriteRequest(req)

				resp, _ := conn.ReadResponse()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				Expect(fakeReporter.CaptureHTTPLatencyCallCount()).ToNot(Equal(0))
			})
		})

		Context("when the endpoint is nil", func() {
			removeAllEndpoints := func(pool *route.EndpointPool) {
				endpoints := make([]*route.Endpoint, 0)
				pool.Each(func(e *route.Endpoint) {
					endpoints = append(endpoints, e)
				})
				for _, e := range endpoints {
					pool.Remove(e)
				}
			}

			metricsNilEndpointsTest := func(expectedStatusCode, expectedBadRequestCallCount int) {
				ln := test_util.RegisterConnHandler(r, "nil-endpoint", func(conn *test_util.HttpConn) {
					conn.CheckLine("GET / HTTP/1.1")
					resp := test_util.NewResponse(http.StatusOK)
					conn.WriteResponse(resp)
					conn.Close()
				})
				defer ln.Close()

				removeAllEndpoints(r.Lookup(route.Uri("nil-endpoint")))
				conn := dialProxy(proxyServer)

				req := test_util.NewRequest("GET", "nil-endpoint", "/", nil)
				conn.WriteRequest(req)

				res, _ := conn.ReadResponse()
				Expect(res.StatusCode).To(Equal(expectedStatusCode))
				Expect(fakeReporter.CaptureBadRequestCallCount()).To(Equal(expectedBadRequestCallCount))
				Expect(fakeReporter.CaptureRoutingResponseCallCount()).To(Equal(0))
				Expect(fakeReporter.CaptureRoutingResponseLatencyCallCount()).To(Equal(0))
			}

			Context("when emptyPoolResponseCode503 is true", func() {
				BeforeEach(func() {
					conf.EmptyPoolResponseCode503 = true
				})

				It("captures neither bad gateway nor routing response", func() {
					metricsNilEndpointsTest(http.StatusServiceUnavailable, 0)
				})
			})

			Context("when emptyPoolResponseCode503 is false", func() {
				BeforeEach(func() {
					conf.EmptyPoolResponseCode503 = false
				})

				It("captures neither bad gateway nor routing response", func() {
					metricsNilEndpointsTest(http.StatusNotFound, 1)
				})
			})
		})
	})
})

// HACK: this is used to silence any http warnings in logs
// that clutter stdout/stderr when running unit tests
func readResponseNoErrorCheck(conn *test_util.HttpConn) *http.Response {
	log.SetOutput(io.Discard)
	resp, err := http.ReadResponse(conn.Reader, &http.Request{})
	Expect(err).ToNot(HaveOccurred())
	log.SetOutput(os.Stderr)
	return resp
}

func dialProxy(proxyServer net.Listener) *test_util.HttpConn {
	conn, err := net.Dial("tcp", proxyServer.Addr().String())
	Expect(err).NotTo(HaveOccurred())

	return test_util.NewHttpConn(conn)
}

func wsClient(conn net.Conn, urlStr string) (*websocket.Conn, error) {
	wsUrl, err := url.ParseRequestURI(urlStr)
	Expect(err).NotTo(HaveOccurred())

	cfg := &websocket.Config{
		Location: wsUrl,
		Origin:   wsUrl,
		Version:  websocket.ProtocolVersionHybi13,
	}

	wsConn, err := websocket.NewClient(cfg, conn)
	return wsConn, err
}

func parseResponseTimeFromLog(log string) float64 {
	r, err := regexp.Compile(`response_time:(\d+.\d+)`)
	Expect(err).ToNot(HaveOccurred())

	responseTimeStr := r.FindStringSubmatch(log)

	f, err := strconv.ParseFloat(responseTimeStr[1], 64)
	Expect(err).ToNot(HaveOccurred())

	return f
}
