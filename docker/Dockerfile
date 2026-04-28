FROM debian:stable-slim

COPY torii /torii

VOLUME /data

WORKDIR /data

EXPOSE 27000

ENTRYPOINT ["/torii"]
CMD ["--config", "/etc/torii/config.yaml", "--data-dir", "/data"]
