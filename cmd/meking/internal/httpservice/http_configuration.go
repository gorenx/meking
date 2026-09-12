package httpservice

// httpConfiguration is derived from the single Project configuration before
// the listener opens and is immutable for the lifetime of the service process.
type httpConfiguration struct {
	ReportVectorsEnabled bool
}
