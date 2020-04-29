FROM golang:1.14-alpine

ADD . /code

RUN set -x \
    && ( \
        cd /code \
        && go build -o telegram-notify . \
    )


FROM alpine:latest
LABEL maintainer "Rafael Martins <rafael@rafaelmartins.eng.br>"

COPY --from=0 /code/telegram-notify /usr/local/bin/telegram-notify

ENTRYPOINT ["/usr/local/bin/telegram-notify"]
