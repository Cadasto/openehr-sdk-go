# help.mk renders `make help` from annotations in the Makefiles make reads.
#
#   ##@ Section               opens a section of targets
#   ##@ Section | note        the same, with a note at the right of the heading
#   target: deps ## Text      lists the target with a one-line description
#   ##= NAME=value | note     adds a line to the Variables block
#
# A target with no "## " is not listed: an internal step, or an old name kept
# as an alias. Sections print in this order, whatever their order in the
# files; a section with another name prints after them, and an empty one is
# left out:
#
#   Setup Build Code Test Generate Vendor Gates Stack Run Docs
#
# Set before `make help` runs: HELP_TITLE and HELP_TOOLCHAIN (the two header
# lines) and HELP_FOOTER (the closing line); none may hold a single quote.
# Colour is used only when the output is a terminal and NO_COLOR is unset.
# Names fit a 20-character column and descriptions about 56 characters, so a
# line fits 80 columns.

.DEFAULT_GOAL := help

HELP_SECTIONS := Setup Build Code Test Generate Vendor Gates Stack Run Docs

define HELP_AWK
function paint(code, text) { return color ? "\033[" code "m" text "\033[0m" : text }
function cut_note(text,   at) {
	note = ""
	if ((at = index(text, " | ")) > 0) { note = substr(text, at + 3); text = substr(text, 1, at - 1) }
	return text
}
function open_section(name) { if (!(name in seen)) { seen[name] = 1; opened[++sections] = name } }
function print_section(name,   k) {
	printf "\n%s", paint("1", name)
	if (name in notes) printf "%s", paint("2", sprintf("%" (80 - length(name)) "s", notes[name]))
	printf "\n"
	for (k = 1; k <= count[name]; k++)
		printf "  %s  %s\n", paint("36", sprintf("%-20s", target[name, k])), about[name, k]
}
BEGIN { ranks = split(order, ranked, " ") }
/^##@ / { current = cut_note(substr($$0, 5)); open_section(current); if (note != "") notes[current] = note; next }
/^##= / { vars[++nvars] = cut_note(substr($$0, 5)); var_note[nvars] = note; next }
/^[A-Za-z0-9][A-Za-z0-9_.%-]*:([^=]|$$)/ && / ## / {
	if (current == "") { current = "Other"; open_section(current) }
	n = ++count[current]
	target[current, n] = substr($$0, 1, index($$0, ":") - 1)
	about[current, n] = substr($$0, index($$0, " ## ") + 4)
}
END {
	if (title != "") print paint("1", title)
	if (toolchain != "") print paint("2", toolchain)
	for (r = 1; r <= ranks; r++) { done[ranked[r]] = 1; if (count[ranked[r]]) print_section(ranked[r]) }
	for (s = 1; s <= sections; s++) if (!(opened[s] in done) && count[opened[s]]) print_section(opened[s])
	if (nvars) {
		printf "\n%s\n", paint("1", "Variables")
		for (k = 1; k <= nvars; k++) printf "  %-42s  %s\n", vars[k], paint("2", var_note[k])
	}
	if (footer != "") printf "\n%s\n", footer
}
endef

.PHONY: help
help: export HELP_PROGRAM = $(HELP_AWK)
help:
	@if [ -t 1 ] && [ -z "$${NO_COLOR:-}" ]; then color=1; else color=0; fi; \
	awk -v color="$$color" -v order='$(HELP_SECTIONS)' -v title='$(HELP_TITLE)' \
	    -v toolchain='$(HELP_TOOLCHAIN)' -v footer='$(HELP_FOOTER)' \
	    "$$HELP_PROGRAM" $(MAKEFILE_LIST)
