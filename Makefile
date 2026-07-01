APPS ?= $(shell find ./apps -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)
SIZES := 1KB 10KB 100KB 300KB
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GOARM := v9.5
GOAMD := v4

LIMIT := 128
ITERATIONS := 8
PARALLELISM := 1

# Idle break inserted between sequential benchmark runs (all-sizes-seq).
# A long enough break leaves a detectable gap so the cloudwatch-aggregate app
# can separate runs and isolate per-step host CPU usage. Uses GNU sleep syntax.
BREAK ?= 10m

SAM_TEMPLATE ?= template.yaml
SAM_STACK_NAME ?= attributevalue-benchmarks
SAM_REGION ?= eu-west-1
SAM_S3_BUCKET ?= attributevalue-benchmarks
SSH_KEY ?= $(HOME)/.ssh/radu-dax.pem
SSH_USER ?= ec2-user
REMOTE_DIR ?= /home/ec2-user/attributevalue-benchmarks

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
		SIZE=$(2) go run . > ../../results/$(1)_$(2)_$(shell date +%Y_%m_%d_%H_%M_%S).txt
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

# Run every app×size one at a time with an idle BREAK between runs. Running them
# sequentially keeps host CPU usage attributable to a single step. Each app emits
# RunMarker datapoints (Phase=start/end) into its own CloudWatch namespace to
# bracket its run, so the cloudwatch-aggregate app can pin the exact window of
# each run and auto-derive the idle gap; the break leaves a visible boundary as a
# fallback signal.
.PHONY: all-sizes-seq
all-sizes-seq: ensure-gvm
	@first=1; \
	for app in $(APPS); do \
		for size in $(SIZES); do \
			if [ $$first -eq 0 ]; then \
				echo "Sleeping $(BREAK) before next run..."; \
				sleep $(BREAK); \
			fi; \
			first=0; \
			TS=$$(date +%Y_%m_%d_%H_%M_%S); \
			echo "Running $$app / $$size..."; \
			cd apps/$$app && \
				LIMIT=$(LIMIT) ITERATIONS=$(ITERATIONS) PARALLELISM=$(PARALLELISM) \
				SIZE=$$size go run . > ../../results/$${app}_$${size}_$${TS}.txt; \
			cd ../..; \
		done; \
	done

# Benchmark each app
.PHONY: gobm
gobm: ensure-gvm
	TIMESTAMP=$$(date +%Y_%m_%d_%H_%M_%S); \
	ARCH=$$(go env GOARCH); \
	for app in $(APPS); do \
		echo "Benchmarking $$app..."; \
		cd apps/$$app && go test -bench=. -benchmem -benchtime=60s -run=notest -count=1 ./... > "../../results/$${app}_bench_$${TIMESTAMP}_$${ARCH}.txt" 2>&1; \
		cd ../..; \
	done

.PHONY: deploy-ec2
deploy-ec2:
	@set -e; \
	if [ -n "$(SAM_S3_BUCKET)" ]; then \
		BUCKET="$(SAM_S3_BUCKET)"; \
	else \
		BUCKET="attributevalue-benchmarks-$(SAM_REGION)"; \
	fi; \
	echo "Using SAM artifact bucket: $$BUCKET"; \
	if ! aws s3api head-bucket --bucket "$$BUCKET" 2>/dev/null; then \
		echo "Creating bucket $$BUCKET in $(SAM_REGION)..."; \
		if [ "$(SAM_REGION)" = "us-east-1" ]; then \
			aws s3api create-bucket --bucket "$$BUCKET" --region "$(SAM_REGION)" >/dev/null; \
		else \
			aws s3api create-bucket --bucket "$$BUCKET" --region "$(SAM_REGION)" --create-bucket-configuration LocationConstraint="$(SAM_REGION)" >/dev/null; \
		fi; \
	fi; \
	sam deploy \
		--template-file $(SAM_TEMPLATE) \
		--stack-name $(SAM_STACK_NAME) \
		--region $(SAM_REGION) \
		--s3-bucket "$$BUCKET" \
		--no-confirm-changeset

.PHONY: sync-ec2
sync-ec2:
	@set -e; \
	IPS=$$(aws cloudformation describe-stacks \
		--stack-name "$(SAM_STACK_NAME)" \
		--region "$(SAM_REGION)" \
		--query "Stacks[0].Outputs[?OutputKey=='Arm64PublicIp' || OutputKey=='Amd64PublicIp'].OutputValue" \
		--output text); \
	if [ -z "$$IPS" ] || [ "$$IPS" = "None" ]; then \
		echo "No instance IPs found in stack outputs (Arm64PublicIp/Amd64PublicIp)."; \
		exit 1; \
	fi; \
	for ip in $$IPS; do \
		echo "Syncing to $$ip..."; \
		rsync -avz --delete \
			--exclude='results/' \
			-e "ssh -i $(SSH_KEY)" ./ "$(SSH_USER)@$$ip:$(REMOTE_DIR)"; \
	done

.PHONY: get-ips
get-ips:
	@set -e; \
	IPS=$$(aws cloudformation describe-stacks \
		--stack-name "$(SAM_STACK_NAME)" \
		--region "$(SAM_REGION)" \
		--query "Stacks[0].Outputs[?OutputKey=='Arm64PublicIp' || OutputKey=='Amd64PublicIp'].OutputValue" \
		--output text); \
	if [ -z "$$IPS" ] || [ "$$IPS" = "None" ]; then \
		echo "No instance IPs found in stack outputs (Arm64PublicIp/Amd64PublicIp)."; \
		exit 1; \
	fi; \
	for ip in $$IPS; do \
		echo "Instance IP: $$ip"; \
	done

.PHONY: setup-ec2
setup-ec2:
	@set -e;\
	IPS=$$(aws cloudformation describe-stacks \
		--stack-name "$(SAM_STACK_NAME)" \
		--region "$(SAM_REGION)" \
		--query "Stacks[0].Outputs[?OutputKey=='Arm64PublicIp' || OutputKey=='Amd64PublicIp'].OutputValue" \
		--output text); \
	if [ -z "$$IPS" ] || [ "$$IPS" = "None" ]; then \
		echo "No instance IPs found in stack outputs (Arm64PublicIp/Amd64PublicIp)."; \
		exit 1; \
	fi; \
	for ip in $$IPS; do \
		echo "Instance IP: $$ip"; \
		ssh -i $(SSH_KEY) "$(SSH_USER)@$$ip" "sudo yum update -y && sudo yum install -y bison git gcc make"; \
		ssh -i $(SSH_KEY) "$(SSH_USER)@$$ip" "curl -fsSL https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer | bash"; \
		ssh -i $(SSH_KEY) "$(SSH_USER)@$$ip" "source /home/ec2-user/.gvm/scripts/gvm && gvm install go1.26.0 -B && gvm use go1.26.0 --default"; \
	done
