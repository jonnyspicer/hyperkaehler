# Use an official Go runtime as the base image
FROM golang:1.21

# Set the working directory to /app
WORKDIR /hyperkaehler

# Copy the current directory contents into the container at /app
COPY . /hyperkaehler

# Download dependencies
RUN go mod download

# Build the Go application
RUN go build -o hyperkaehler

# Expose port 8080 to the outside world
EXPOSE 8080
EXPOSE 80
EXPOSE 443

# Run the application when the container starts
CMD ["./hyperkaehler"]