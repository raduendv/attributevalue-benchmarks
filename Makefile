APPS ?= $(shell find ./apps -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)
SIZES := 1KB 10KB 100KB 300KB
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GOARM := v9.5
GOAMD := v4

LIMIT := 128
ITERATIONS := 8
PARALLELISM := 1

#APP_V1_GO_VERSION := go1.26.0
#APP_V2_GO_VERSION := go1.26.0
#APP_V2-PTR_GO_VERSION := go1.26.0
#APP_V2-CODEGEN_GO_VERSION := go1.26.0

# Running make -j4 -B will execute all apps in parallel (change the number to match the numbers of apps you want running in parallel)
.PHONY: all
all: $(APPS)

# Expand every app×size combo as an independent target, e.g. v1/1KB, v2/10KB
ALL_SIZE_TARGETS := $(foreach app,$(APPS),$(foreach size,$(SIZES),$(app)/$(size)))

define run_app_size
.PHONY: $(1)/$(2)
$(1)/$(2): ensure-gvm
	cd apps/$(1) && \
		LIMIT=$(LIMIT) ITERATIONS=$(ITERATIONS) PARALLELISM=$(PARALLELISM) \
		PAYLOAD_SIZE=$(2) SIZE=$(2) go run . > ../../results/$(1)_$(2)_$(shell date +%Y_%m_%d_%H_%M_%S).txt
endef

$(foreach app,$(APPS),$(foreach size,$(SIZES),$(eval $(call run_app_size,$(app),$(size)))))

.PHONY: ensure-gvm
ensure-gvm:
	@if [ ! -s "$$HOME/.gvm/scripts/gvm" ]; then \
		echo "GVM not installed. Installing..."; \
		curl -fsSL https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer | bash; \
		echo "GVM install complete. Restart your shell or run 'source $$HOME/.gvm/scripts/gvm' to use it in this session."; \
		exit 1; \
	else \
		echo "GVM exists"; \
	fi

.PHONY: $(APPS)
$(APPS): ensure-gvm
	@cd apps/$@ && \
		LIMIT=$(LIMIT) ITERATIONS=$(ITERATIONS) go run *.go
#	$(eval UC := $(shell echo '$@' | tr '[:lower:]' '[:upper:]'))
#	$(eval GO_VERSION := $(shell echo 'APP_$(UC)_GO_VERSION'))
#	@. "$$HOME/.gvm/scripts/gvm"; \
#	gvm use $(APP_$(UC)_GO_VERSION); \
#	cd apps/$@ && go mod tidy && cd ../..; \
#	go run apps/$@/main.go

# Recommended -j16 or higher for running all apps in parallel, depending on your CPU cores
.PHONY: all-sizes
all-sizes: $(ALL_SIZE_TARGETS)
