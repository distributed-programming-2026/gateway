# mise run build produces the Linux binary.
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/gateway /gateway
EXPOSE 8000 15000 8082
ENTRYPOINT ["/gateway"]
CMD ["--config", "/config"]
