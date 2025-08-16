package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualService represents an Istio VirtualService resource
type VirtualService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualServiceSpec `json:"spec,omitempty"`
}

// VirtualServiceSpec defines the desired state of VirtualService
type VirtualServiceSpec struct {
	Hosts    []string    `json:"hosts,omitempty"`
	Gateways []string    `json:"gateways,omitempty"`
	HTTP     []HTTPRoute `json:"http,omitempty"`
	TCP      []TCPRoute  `json:"tcp,omitempty"`
	TLS      []TLSRoute  `json:"tls,omitempty"`
	ExportTo []string    `json:"exportTo,omitempty"`
}

// HTTPRoute describes HTTP routes
type HTTPRoute struct {
	Match    []HTTPMatchRequest     `json:"match,omitempty"`
	Route    []HTTPRouteDestination `json:"route,omitempty"`
	Redirect *HTTPRedirect          `json:"redirect,omitempty"`
	Rewrite  *HTTPRewrite           `json:"rewrite,omitempty"`
	Timeout  *time.Duration         `json:"timeout,omitempty"`
	Retries  *HTTPRetry             `json:"retries,omitempty"`
	Fault    *HTTPFaultInjection    `json:"fault,omitempty"`
	Mirror   *Destination           `json:"mirror,omitempty"`
}

// TCPRoute describes TCP routes
type TCPRoute struct {
	Match []L4MatchAttributes `json:"match,omitempty"`
	Route []RouteDestination  `json:"route,omitempty"`
}

// TLSRoute describes TLS routes
type TLSRoute struct {
	Match []TLSMatchAttributes `json:"match,omitempty"`
	Route []RouteDestination   `json:"route,omitempty"`
}

// HTTPMatchRequest criteria for matching HTTP requests
type HTTPMatchRequest struct {
	URI          *StringMatch            `json:"uri,omitempty"`
	Scheme       *StringMatch            `json:"scheme,omitempty"`
	Method       *StringMatch            `json:"method,omitempty"`
	Authority    *StringMatch            `json:"authority,omitempty"`
	Headers      map[string]*StringMatch `json:"headers,omitempty"`
	Port         *uint32                 `json:"port,omitempty"`
	SourceLabels map[string]string       `json:"sourceLabels,omitempty"`
	Gateways     []string                `json:"gateways,omitempty"`
}

// L4MatchAttributes describes L4 match attributes
type L4MatchAttributes struct {
	DestinationSubnets []string          `json:"destinationSubnets,omitempty"`
	Port               *uint32           `json:"port,omitempty"`
	SourceLabels       map[string]string `json:"sourceLabels,omitempty"`
	Gateways           []string          `json:"gateways,omitempty"`
}

// TLSMatchAttributes describes TLS match attributes
type TLSMatchAttributes struct {
	SNIHosts           []string          `json:"sniHosts,omitempty"`
	DestinationSubnets []string          `json:"destinationSubnets,omitempty"`
	Port               *uint32           `json:"port,omitempty"`
	SourceLabels       map[string]string `json:"sourceLabels,omitempty"`
	Gateways           []string          `json:"gateways,omitempty"`
}

// HTTPRouteDestination describes HTTP route destinations
type HTTPRouteDestination struct {
	Destination *Destination `json:"destination,omitempty"`
	Weight      *int32       `json:"weight,omitempty"`
	Headers     *Headers     `json:"headers,omitempty"`
}

// RouteDestination describes route destinations for TCP/TLS
type RouteDestination struct {
	Destination *Destination `json:"destination,omitempty"`
	Weight      *int32       `json:"weight,omitempty"`
}

// Destination describes a service destination
type Destination struct {
	Host   string        `json:"host,omitempty"`
	Subset string        `json:"subset,omitempty"`
	Port   *PortSelector `json:"port,omitempty"`
}

// PortSelector describes how to select a port
type PortSelector struct {
	Number *uint32 `json:"number,omitempty"`
	Name   string  `json:"name,omitempty"`
}

// StringMatch describes string matching criteria
type StringMatch struct {
	Exact  string `json:"exact,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
}

// Headers describes header manipulation
type Headers struct {
	Request  *HeaderOperations `json:"request,omitempty"`
	Response *HeaderOperations `json:"response,omitempty"`
}

// HeaderOperations describes header operations
type HeaderOperations struct {
	Set    map[string]string `json:"set,omitempty"`
	Add    map[string]string `json:"add,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

// HTTPRedirect describes HTTP redirect
type HTTPRedirect struct {
	URI          string  `json:"uri,omitempty"`
	Authority    string  `json:"authority,omitempty"`
	RedirectCode *uint32 `json:"redirectCode,omitempty"`
}

// HTTPRewrite describes HTTP rewrite
type HTTPRewrite struct {
	URI       string `json:"uri,omitempty"`
	Authority string `json:"authority,omitempty"`
}

// HTTPRetry describes HTTP retry policy
type HTTPRetry struct {
	Attempts      *int32         `json:"attempts,omitempty"`
	PerTryTimeout *time.Duration `json:"perTryTimeout,omitempty"`
	RetryOn       string         `json:"retryOn,omitempty"`
}

// HTTPFaultInjection describes HTTP fault injection
type HTTPFaultInjection struct {
	Delay *HTTPFaultInjectionDelay `json:"delay,omitempty"`
	Abort *HTTPFaultInjectionAbort `json:"abort,omitempty"`
}

// HTTPFaultInjectionDelay describes delay fault injection
type HTTPFaultInjectionDelay struct {
	Percentage *Percentage    `json:"percentage,omitempty"`
	FixedDelay *time.Duration `json:"fixedDelay,omitempty"`
}

// HTTPFaultInjectionAbort describes abort fault injection
type HTTPFaultInjectionAbort struct {
	Percentage *Percentage `json:"percentage,omitempty"`
	HTTPStatus *int32      `json:"httpStatus,omitempty"`
}

// Percentage describes a percentage value
type Percentage struct {
	Value float64 `json:"value,omitempty"`
}
