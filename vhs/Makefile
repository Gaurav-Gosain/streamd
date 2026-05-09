# Build every GIF under vhs/out/ from the checked-in tape files.
# Run from the repo root:
#
#   make -C vhs            # build all gifs
#   make -C vhs gif-hero   # build a single one
#   make -C vhs clean      # drop generated gifs
#
# Requires `vhs` (github.com/charmbracelet/vhs) and `jq` on $PATH.
# A fresh `streamd` binary is built into ../bin and prepended to
# $PATH during each vhs run, so the gifs always match the source.

VHS     ?= vhs
GO      ?= go
ROOT    := $(abspath ..)
BIN_DIR := $(ROOT)/bin
TAPES   := $(wildcard *.tape)
GIFS    := $(patsubst %.tape,out/%.gif,$(filter-out _common.tape,$(TAPES)))

.PHONY: all build clean list $(patsubst %.tape,gif-%,$(filter-out _common.tape,$(TAPES)))

all: build $(GIFS)

build:
	@mkdir -p $(BIN_DIR)
	@echo "  build streamd"
	@cd $(ROOT) && $(GO) build -o bin/streamd .
	@echo "  PATH will include $(BIN_DIR) during vhs runs"

out/%.gif: %.tape _common.tape fixtures/*
	@mkdir -p out
	@echo "  vhs  $<"
	@cd $(ROOT) && PATH="$(BIN_DIR):$$PATH" $(VHS) vhs/$<

gif-%: %.tape
	@mkdir -p out
	@echo "  vhs  $<"
	@cd $(ROOT) && PATH="$(BIN_DIR):$$PATH" $(VHS) vhs/$<

clean:
	rm -f out/*.gif

list:
	@printf "  %s\n" $(TAPES)
