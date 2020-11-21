package libhttp

import "fmt"

func HSTSFilter(maxAge int) func(request Request, service Service) Response {
	return func(request Request, service Service) Response {
		response := service(request)
		response.Header.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d; includeSubDomains", maxAge))
		return response
	}
}
