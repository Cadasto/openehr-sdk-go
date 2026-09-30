// Package a has files this machine leaves out of its build. Scan and Imports
// must read the ones another build compiles, and skip the rest.
package a

import _ "strings"
