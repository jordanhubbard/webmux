# Delegate all public targets to the repository Makefile, including future ones.
# Keep the default explicit so invoking make without arguments still builds.
.DEFAULT_GOAL := all

.PHONY: all
all:
	@$(MAKE) --no-print-directory -C "$(CURDIR)/.." all

# Never let GNU Make's implicit remake of this file invoke the parent build.
Makefile: ;

%:
	@$(MAKE) --no-print-directory -C "$(CURDIR)/.." $@
