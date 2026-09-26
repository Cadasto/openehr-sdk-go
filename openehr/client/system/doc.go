// Package system is the openEHR REST 1.1.0-development System API
// client. It exposes the deployment's service capabilities, declared
// spec version, and a coarse health probe.
//
// The System API is the smallest typed leaf client in the SDK. It
// exercises the full stack (service-catalog routing, the transport, and
// error mapping) without RM polymorphism, optimistic concurrency, or
// content negotiation. Call system.Capabilities once at startup to
// confirm the deployment's declared spec version matches the ITS-REST
// version the SDK targets.
//
// The openEHR REST OpenAPI YAML at
// github.com/openEHR/specifications-ITS-REST is authoritative for
// endpoint shapes. ServiceCapabilities documents the standard fields;
// deployment-specific fields are preserved verbatim in Extras.
package system
