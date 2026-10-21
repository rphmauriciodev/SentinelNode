FROM alpine:3.22

RUN apk add --no-cache openssl bash iproute2

WORKDIR /certs

COPY generate_certs.sh /usr/local/bin/generate_certs.sh
RUN chmod +x /usr/local/bin/generate_certs.sh

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME ["/certs"]

ENTRYPOINT ["/entrypoint.sh"]
