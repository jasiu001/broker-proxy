FROM alpine:3.10

RUN apk --no-cache add ca-certificates
RUN apk add --no-cache curl

COPY ./broker-proxy /root/broker-proxy

ENTRYPOINT ["/root/broker-proxy"]
