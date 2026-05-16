// Package system is the openEHR REST 1.1.0-development System API
// client. It exposes the deployment's service capabilities, declared
// spec version, and a coarse health probe.
//
// The System API is the smallest typed leaf client in the SDK. It
// validates the full stack — service-catalog routing (REQ-070), the
// transport (REQ-021..026, REQ-090), and error mapping (REQ-093) —
// without RM polymorphism, optimistic concurrency, or content
// negotiation. Consumers SHOULD call system.Capabilities once at
// startup to confirm the deployment's declared spec version matches
// the SDK's pinned target.
//
// Wire authority: per REQ-095 the openEHR REST OpenAPI YAML at
// github.com/openEHR/specifications-ITS-REST is authoritative for
// endpoint shapes. ServiceCapabilities documents the standard fields;
// deployment-specific fields are preserved verbatim in Extras.
package system
