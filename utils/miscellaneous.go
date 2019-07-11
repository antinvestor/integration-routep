package utils

import (
	"net"
	"net/http"
	"os"
)

func GetIp(r *http.Request) string  {
	sourceIp := r.Header.Get("X-FORWARDED-FOR")
	if sourceIp == ""{
		sourceIp,_,_ = net.SplitHostPort(r.RemoteAddr)
	}

	return sourceIp
}

// GetEnv Obtains the environment key or returns the default value
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
