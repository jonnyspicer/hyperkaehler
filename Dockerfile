# Use an official Go runtime as the base image
FROM golang:1.20-alpine

# Set the working directory to /app
WORKDIR /hyperkaehler

# Copy the current directory contents into the container at /app
COPY . /hyperkaehler

# Install Git
RUN apk update && apk add --no-cache git

# Workaround for networks with the usual Go package proxy blocked
RUN go env -w GOPROXY=direct

# Install the mango package
RUN go get github.com/jonnyspicer/mango

# Build the Go application
RUN go build -o hyperkaehler

# Expose port 8080 to the outside world
EXPOSE 8080
EXPOSE 80
EXPOSE 443

# Run the application when the container starts
CMD ["./hyperkaehler"]