FROM alpine

#ADD ca-certificates.crt /etc/ssl/certs/
RUN apk --no-cache add --update ca-certificates \
 && rm -rf /var/cache/apk/*

COPY ./linux-go-api-example /app/goserver

ARG PORT
ENV PORT ${PORT:-8081}

# Run as non root user
RUN addgroup -g 10001 -S app && \
    adduser -u 10001 -S app -G app 
USER app

EXPOSE ${PORT}

CMD ["/app/goserver"]
