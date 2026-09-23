# Used by `make pack-client`: runs the prebuilt binary, which has the
# dashboard built in. The root Dockerfile builds from source instead.
FROM alpine:3.22
RUN adduser -D -H symon
WORKDIR /app
COPY client_linux_x86_64 ./client
USER symon
EXPOSE 8080
CMD ["./client"]
