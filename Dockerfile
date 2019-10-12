FROM scratch

COPY service/hasty_service /

CMD ["/hasty_service"]
