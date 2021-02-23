generate_proto:
	@docker run -v `pwd`:/defs namely/protoc-all -f proto/helloworld.proto -l java

build:
	@docker build --no-cache -t vhive-spring-function .

run:
	@docker run --rm -it vhive-spring-function

build_and_run: build run