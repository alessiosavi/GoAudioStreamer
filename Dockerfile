# Stage 1: Use a minimal base image
FROM ubuntu:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the pre-compiled binary from your local system 
# (assuming your binary is named 'server')
COPY bin/server /app/server

# Set permissions to ensure the binary is executable
RUN chmod +x /app/server

# Expose the port your gRPC server listens on (e.g., 50051)
# NOTE: This is for documentation; it doesn't actually publish the port.
EXPOSE 50051 
EXPOSE 9091 

# Command to execute the binary when the container starts
CMD ["/app/server"]