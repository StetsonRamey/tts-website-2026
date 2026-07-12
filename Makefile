.PHONY: dev build build-prod clean

dev:
	hugo server --disableFastRender

build:
	hugo

build-prod:
	hugo --minify --gc --environment production --baseURL 'https://tistheseasonkc.com/'

clean:
	rm -rf resources/_gen public
