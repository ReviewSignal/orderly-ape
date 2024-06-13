.PHONY: all

all: requirements.txt requirements.dev.txt requirements.docker.txt

requirements.txt: pyproject.toml
	uv pip compile --generate-hashes $< > $@

requirements.dev.txt: pyproject.toml requirements.txt
	uv pip compile -c requirements.txt --extra dev --generate-hashes $< > $@

requirements.docker.txt: pyproject.toml requirements.txt
	uv pip compile -c requirements.txt --extra docker --generate-hashes $< > $@

