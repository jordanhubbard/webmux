# Delegate to the top-level Makefile.
# Run `make` from the repository root for full usage and options.

.PHONY: all build install start stop restart status test lint clean help

all build install start stop restart status test lint clean help:
	@$(MAKE) --no-print-directory -C $(CURDIR)/.. $@
