FROM golang:1.8

RUN apt-get update \
    && apt-get install -y curl python-software-properties \
    && apt-get -y autoclean

RUN curl -sL https://deb.nodesource.com/setup_9.x | bash -

RUN apt-get install -y nodejs

WORKDIR /go/src/app
COPY . .

RUN make install
RUN make

CMD ["out/retro"]
