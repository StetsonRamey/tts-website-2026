.PHONY: dev build build-prod clean

dev:
	hugo server --disableFastRender

build:
	hugo

build-prod:
	hugo --minify --gc

clean:
	rm -rf resources/_gen public
