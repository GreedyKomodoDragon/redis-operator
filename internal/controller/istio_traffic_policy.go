package controller

import (
	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// trafficPolicyBuilder helps build Istio traffic policies with reduced complexity
type trafficPolicyBuilder struct {
	policy *koncachev1alpha1.RedisIstioTrafficPolicy
	result map[string]interface{}
}

// newTrafficPolicyBuilder creates a new traffic policy builder
func newTrafficPolicyBuilder(policy *koncachev1alpha1.RedisIstioTrafficPolicy) *trafficPolicyBuilder {
	return &trafficPolicyBuilder{
		policy: policy,
		result: make(map[string]interface{}),
	}
}

// build constructs the traffic policy
func (tpb *trafficPolicyBuilder) build() map[string]interface{} {
	tpb.addLoadBalancer()
	tpb.addConnectionPool()
	tpb.addOutlierDetection()
	return tpb.result
}

// addLoadBalancer adds load balancer configuration
func (tpb *trafficPolicyBuilder) addLoadBalancer() {
	if tpb.policy.LoadBalancer == nil {
		return
	}

	lb := make(map[string]interface{})
	if tpb.policy.LoadBalancer.Simple != "" {
		lb["simple"] = tpb.policy.LoadBalancer.Simple
	}

	if tpb.policy.LoadBalancer.ConsistentHash != nil {
		ch := tpb.buildConsistentHash()
		if len(ch) > 0 {
			lb["consistentHash"] = ch
		}
	}

	if len(lb) > 0 {
		tpb.result["loadBalancer"] = lb
	}
}

// buildConsistentHash builds consistent hash configuration
func (tpb *trafficPolicyBuilder) buildConsistentHash() map[string]interface{} {
	ch := tpb.policy.LoadBalancer.ConsistentHash
	result := make(map[string]interface{})

	if ch.HTTPHeaderName != "" {
		result["httpHeaderName"] = ch.HTTPHeaderName
	}

	if ch.UseSourceIP {
		result["useSourceIp"] = true
	}

	if ch.HTTPCookie != nil {
		cookie := map[string]interface{}{
			"name": ch.HTTPCookie.Name,
		}
		if ch.HTTPCookie.TTL != nil {
			cookie["ttl"] = ch.HTTPCookie.TTL.Duration.String()
		}
		result["httpCookie"] = cookie
	}

	return result
}

// addConnectionPool adds connection pool configuration
func (tpb *trafficPolicyBuilder) addConnectionPool() {
	if tpb.policy.ConnectionPool == nil {
		return
	}

	cp := make(map[string]interface{})

	if tpb.policy.ConnectionPool.TCP != nil {
		tcp := tpb.buildTCPSettings()
		if len(tcp) > 0 {
			cp["tcp"] = tcp
		}
	}

	if tpb.policy.ConnectionPool.HTTP != nil {
		http := tpb.buildHTTPSettings()
		if len(http) > 0 {
			cp["http"] = http
		}
	}

	if len(cp) > 0 {
		tpb.result["connectionPool"] = cp
	}
}

// buildTCPSettings builds TCP connection settings
func (tpb *trafficPolicyBuilder) buildTCPSettings() map[string]interface{} {
	tcp := tpb.policy.ConnectionPool.TCP
	result := make(map[string]any)

	if tcp.MaxConnections > 0 {
		result["maxConnections"] = int64(tcp.MaxConnections)
	}
	if tcp.ConnectTimeout != nil {
		result["connectTimeout"] = tcp.ConnectTimeout.Duration.String()
	}
	if tcp.TCPNoDelay {
		result["tcpNoDelay"] = true
	}
	if tcp.KeepaliveTime != nil {
		result["keepaliveTime"] = tcp.KeepaliveTime.Duration.String()
	}

	return result
}

// buildHTTPSettings builds HTTP connection settings
func (tpb *trafficPolicyBuilder) buildHTTPSettings() map[string]interface{} {
	http := tpb.policy.ConnectionPool.HTTP
	result := make(map[string]interface{})

	if http.HTTP1MaxPendingRequests > 0 {
		result["http1MaxPendingRequests"] = int64(http.HTTP1MaxPendingRequests)
	}
	if http.HTTP2MaxRequests > 0 {
		result["http2MaxRequests"] = int64(http.HTTP2MaxRequests)
	}
	if http.MaxRequestsPerConnection > 0 {
		result["maxRequestsPerConnection"] = int64(http.MaxRequestsPerConnection)
	}
	if http.MaxRetries > 0 {
		result["maxRetries"] = int64(http.MaxRetries)
	}
	if http.IdleTimeout != nil {
		result["idleTimeout"] = http.IdleTimeout.Duration.String()
	}
	if http.H2UpgradePolicy != "" {
		result["h2UpgradePolicy"] = http.H2UpgradePolicy
	}

	return result
}

// addOutlierDetection adds outlier detection configuration
func (tpb *trafficPolicyBuilder) addOutlierDetection() {
	if tpb.policy.OutlierDetection == nil {
		return
	}

	od := make(map[string]interface{})
	outlier := tpb.policy.OutlierDetection

	if outlier.Consecutive5xxErrors > 0 {
		od["consecutive5xxErrors"] = int64(outlier.Consecutive5xxErrors)
	}
	if outlier.ConsecutiveGatewayErrors > 0 {
		od["consecutiveGatewayErrors"] = int64(outlier.ConsecutiveGatewayErrors)
	}
	if outlier.Interval != nil {
		od["interval"] = outlier.Interval.Duration.String()
	}
	if outlier.BaseEjectionTime != nil {
		od["baseEjectionTime"] = outlier.BaseEjectionTime.Duration.String()
	}
	if outlier.MaxEjectionPercent > 0 {
		od["maxEjectionPercent"] = int64(outlier.MaxEjectionPercent)
	}
	if outlier.MinHealthPercent > 0 {
		od["minHealthPercent"] = int64(outlier.MinHealthPercent)
	}

	if len(od) > 0 {
		tpb.result["outlierDetection"] = od
	}
}

// buildTrafficPolicy builds an Istio traffic policy from the Redis configuration
func buildTrafficPolicy(policy *koncachev1alpha1.RedisIstioTrafficPolicy) map[string]interface{} {
	if policy == nil {
		return make(map[string]interface{})
	}
	return newTrafficPolicyBuilder(policy).build()
}
