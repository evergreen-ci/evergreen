#!/bin/bash
sudo rm -rf _out/*
go test binary_test.go
cp _out/dist/*_amd64.deb .
go test source_test.go
cd _out/dist/
dpkg-source -x *.dsc
cd testpkg-0.0.2
sudo dpkg-buildpackage -us -uc


