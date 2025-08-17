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

// UnstructuredTCPRoute represents a TCP route for unstructured conversion
type UnstructuredTCPRoute struct {
	Match   []UnstructuredL4Match     `json:"match,omitempty"`
	Route   []UnstructuredDestination `json:"route,omitempty"`
	Timeout *string                   `json:"timeout,omitempty"`
}

// UnstructuredL4Match represents L4 match for unstructured conversion
type UnstructuredL4Match struct {
	Port               *int64            `json:"port,omitempty"`
	SourceLabels       map[string]string `json:"sourceLabels,omitempty"`
	Gateways           []string          `json:"gateways,omitempty"`
	DestinationSubnets []string          `json:"destinationSubnets,omitempty"`
}

// UnstructuredDestination represents a destination for unstructured conversion
type UnstructuredDestination struct {
	Destination UnstructuredServiceDestination `json:"destination"`
	Weight      *int32                         `json:"weight,omitempty"`
}

// UnstructuredServiceDestination represents service destination for unstructured conversion
type UnstructuredServiceDestination struct {
	Host   string                    `json:"host"`
	Subset string                    `json:"subset,omitempty"`
	Port   *UnstructuredPortSelector `json:"port,omitempty"`
}

// UnstructuredPortSelector represents port selector for unstructured conversion
type UnstructuredPortSelector struct {
	Number *int64 `json:"number,omitempty"`
	Name   string `json:"name,omitempty"`
}

// ToUnstructured converts a structured TCPRoute to an unstructured representation
func (tr *TCPRoute) ToUnstructured(timeout *string) *UnstructuredTCPRoute {
	unstructured := &UnstructuredTCPRoute{
		Match:   make([]UnstructuredL4Match, len(tr.Match)),
		Route:   make([]UnstructuredDestination, len(tr.Route)),
		Timeout: timeout,
	}

	// Convert matches
	for i, match := range tr.Match {
		unstructured.Match[i] = UnstructuredL4Match{
			SourceLabels:       match.SourceLabels,
			Gateways:           match.Gateways,
			DestinationSubnets: match.DestinationSubnets,
		}
		if match.Port != nil {
			port := int64(*match.Port)
			unstructured.Match[i].Port = &port
		}
	}

	// Convert routes
	for i, route := range tr.Route {
		unstructured.Route[i] = UnstructuredDestination{
			Destination: UnstructuredServiceDestination{
				Host:   route.Destination.Host,
				Subset: route.Destination.Subset,
			},
			Weight: route.Weight,
		}
		if route.Destination.Port != nil {
			if route.Destination.Port.Number != nil {
				port := int64(*route.Destination.Port.Number)
				unstructured.Route[i].Destination.Port = &UnstructuredPortSelector{
					Number: &port,
					Name:   route.Destination.Port.Name,
				}
			} else if route.Destination.Port.Name != "" {
				unstructured.Route[i].Destination.Port = &UnstructuredPortSelector{
					Name: route.Destination.Port.Name,
				}
			}
		}
	}

	return unstructured
}

// ToMap converts UnstructuredTCPRoute to map[string]interface{}
func (utr *UnstructuredTCPRoute) ToMap() map[string]interface{} {
	result := make(map[string]interface{})

	// Convert matches
	if len(utr.Match) > 0 {
		result["match"] = utr.convertMatches()
	}

	// Convert routes
	if len(utr.Route) > 0 {
		result["route"] = utr.convertRoutes()
	}

	// Timeout
	if utr.Timeout != nil {
		result["timeout"] = *utr.Timeout
	}

	return result
}

// convertMatches converts match criteria to interface{} slice
func (utr *UnstructuredTCPRoute) convertMatches() []interface{} {
	matches := make([]interface{}, len(utr.Match))
	for i, match := range utr.Match {
		matches[i] = utr.convertSingleMatch(match)
	}
	return matches
}

// convertSingleMatch converts a single match criterion
func (utr *UnstructuredTCPRoute) convertSingleMatch(match UnstructuredL4Match) map[string]interface{} {
	matchMap := make(map[string]interface{})

	if match.Port != nil {
		matchMap["port"] = *match.Port
	}
	if len(match.SourceLabels) > 0 {
		matchMap["sourceLabels"] = match.SourceLabels
	}
	if len(match.Gateways) > 0 {
		matchMap["gateways"] = match.Gateways
	}
	if len(match.DestinationSubnets) > 0 {
		matchMap["destinationSubnets"] = match.DestinationSubnets
	}

	return matchMap
}

// convertRoutes converts route destinations to interface{} slice
func (utr *UnstructuredTCPRoute) convertRoutes() []interface{} {
	routes := make([]interface{}, len(utr.Route))
	for i, route := range utr.Route {
		routes[i] = utr.convertSingleRoute(route)
	}
	return routes
}

// convertSingleRoute converts a single route destination
func (utr *UnstructuredTCPRoute) convertSingleRoute(route UnstructuredDestination) map[string]interface{} {
	routeMap := map[string]interface{}{
		"destination": utr.convertDestination(route.Destination),
	}

	if route.Weight != nil {
		routeMap["weight"] = *route.Weight
	}

	return routeMap
}

// convertDestination converts destination to map
func (utr *UnstructuredTCPRoute) convertDestination(dest UnstructuredServiceDestination) map[string]interface{} {
	destination := map[string]interface{}{
		"host": dest.Host,
	}

	if dest.Subset != "" {
		destination["subset"] = dest.Subset
	}

	if dest.Port != nil {
		if portMap := utr.convertPort(*dest.Port); len(portMap) > 0 {
			destination["port"] = portMap
		}
	}

	return destination
}

// convertPort converts port selector to map
func (utr *UnstructuredTCPRoute) convertPort(port UnstructuredPortSelector) map[string]interface{} {
	portMap := make(map[string]interface{})

	if port.Number != nil {
		portMap["number"] = *port.Number
	}
	if port.Name != "" {
		portMap["name"] = port.Name
	}

	return portMap
}
