FROM scratch

COPY service/hasty_service /

ENTRYPOINT ["/hasty_service"]
