# This is the Dockerfile used by the hadjri/evg-container-self-testing image.
FROM ubuntu:20.04
CMD ["/bin/bash"]
ENV DEBIAN_FRONTEND=noninteractive
RUN apt update
RUN apt install -y curl
RUN apt install -y git
RUN apt install -y make
RUN apt install -y python
RUN apt install -y wget
RUN apt install -y rsync
RUN apt install -y libcurl4
RUN curl -sL https://deb.nodesource.com/setup_14.x -o nodesource_setup.sh
RUN bash nodesource_setup.sh
RUN apt-get install -y nodejs
RUN wget https://go.dev/dl/go1.19.linux-amd64.tar.gz
RUN tar -xvf go1.19.linux-amd64.tar.gz
RUN mv go /usr/local
RUN export GOROOT=/usr/local/go
RUN export PATH=$PATH:/usr/local/go/bin