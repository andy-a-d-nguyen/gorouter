package handlers

import (
	"log/slog"
	"net/http"

	"github.com/urfave/negroni/v3"

	log "code.cloudfoundry.org/gorouter/logger"
)

const (
	VcapRequestIdHeader = "X-Vcap-Request-Id"
)

type setVcapRequestIdHeader struct {
	logger *slog.Logger
}

func NewVcapRequestIdHeader(logger *slog.Logger) negroni.Handler {
	return &setVcapRequestIdHeader{
		logger: logger,
	}
}

func (s *setVcapRequestIdHeader) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// The X-Vcap-Request-Id must be set before the request is passed into the
	// dropsonde InstrumentedHandler

	requestInfo, err := ContextRequestInfo(r)
	if err != nil {
		s.logger.Error("failed-to-get-request-info", log.ErrAttr(err))
		return
	}

	logger := LoggerWithTraceInfo(s.logger, r)

	traceInfo, err := requestInfo.ProvideTraceInfo()
	if err != nil {
		logger.Error("failed-to-get-trace-info", log.ErrAttr(err))
		return
	}

	r.Header.Set(VcapRequestIdHeader, traceInfo.UUID)
	logger.Debug("vcap-request-id-header-set", slog.String("VcapRequestIdHeader", traceInfo.UUID))

	next(rw, r)
}
