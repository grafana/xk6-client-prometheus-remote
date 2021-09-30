FROM ubuntu:xenial

COPY k6 .

ENTRYPOINT [ "./k6" ]