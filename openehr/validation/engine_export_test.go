package validation

// engine_export_test.go: hands the one walk, which no exported entry runs yet,
// to the package's external tests, so they compare it with the exported
// entries over the roots they already load.

// RunEngine runs the one walk over root, with the compiled template c, within
// lim.
var RunEngine = runEngine
