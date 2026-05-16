// Package client groups REST clients per openEHR resource — each
// sub-package corresponds to an openEHR REST 1.1.0-development surface.
//
//   - client/ehr         — EHR and sub-resources (Composition,
//     Contribution, Directory, EHR_STATUS,
//     ItemTags)
//   - client/query       — AQL executor
//   - client/definition  — templates, stored queries
//   - client/demographic — demographic resources
//   - client/system      — system info, capabilities (where applicable)
//
// Clients use generics for shared request/response shapes and accept
// context.Context as the first argument on every I/O method.
package client
