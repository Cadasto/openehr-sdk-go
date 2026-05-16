// Package cadasto is the namespace for Cadasto-platform extras —
// application-specific layers shipped in the same module in v1 for
// adoption convenience.
//
// The cadasto/ subtree is the single cut line for later extraction:
// nothing under openehr/, auth/, smart/, transport/, sandbox/, or
// testkit/ imports from cadasto/…, and no cadasto/<name> imports
// another cadasto/<other> directly — they share through openEHR-core
// types or interface contracts.
//
// See the SDK Specification proposal — research strand on Cadasto
// extras boundary, criteria, and conditional extraction.
package cadasto
