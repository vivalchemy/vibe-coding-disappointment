# A simple justfile with common dev tasks.

# The `default` recipe is special—it runs when you type `just` without an argument.
default:
  just --list

# The `test` recipe runs tests.
# You can use an `@` to suppress the command from being printed to the console.
test:
  @echo "Running tests..."
  go test ./...

# The `build` recipe builds the Go executable.
build:
  @echo "Building the project..."
  go build -o myapp .

# The `run` recipe runs the built application.
# It depends on the `build` recipe, so `just` will run `build` first.
run: build
  @echo "Running the application..."
  ./myapp

# The `clean` recipe removes the build artifacts.
clean:
  @echo "Cleaning up..."
  rm myapp

# A simple justfile with common dev tasks.

# The `default` recipe is special—it runs when you type `just` without an argument.
default:
  just --list

# The `test` recipe runs tests.
# You can use an `@` to suppress the command from being printed to the console.
test:
  @echo "Running tests..."
  go test ./...

# The `build` recipe builds the Go executable.
build:
  @echo "Building the project..."
  go build -o myapp .

# The `run` recipe runs the built application.
# It depends on the `build` recipe, so `just` will run `build` first.
run: build
  @echo "Running the application..."
  ./myapp

# The `clean` recipe removes the build artifacts.
clean:
  @echo "Cleaning up..."
  rm myapp
# A simple justfile with common dev tasks.

# The `default` recipe is special—it runs when you type `just` without an argument.
default:
  just --list

# The `test` recipe runs tests.
# You can use an `@` to suppress the command from being printed to the console.
test:
  @echo "Running tests..."
  go test ./...

# The `build` recipe builds the Go executable.
build:
  @echo "Building the project..."
  go build -o myapp .

# The `run` recipe runs the built application.
# It depends on the `build` recipe, so `just` will run `build` first.
run: build
  @echo "Running the application..."
  ./myapp

# The `clean` recipe removes the build artifacts.
clean:
  @echo "Cleaning up..."
  rm myapp

# A simple justfile with common dev tasks.

# The `default` recipe is special—it runs when you type `just` without an argument.
default:
  just --list

# The `test` recipe runs tests.
# You can use an `@` to suppress the command from being printed to the console.
test:
  @echo "Running tests..."
  go test ./...

# The `build` recipe builds the Go executable.
build:
  @echo "Building the project..."
  go build -o myapp .

# The `run` recipe runs the built application.
# It depends on the `build` recipe, so `just` will run `build` first.
run: build
  @echo "Running the application..."
  ./myapp

# The `clean` recipe removes the build artifacts.
clean:
  @echo "Cleaning up..."
  rm myapp

# A simple justfile with common dev tasks.

# The `default` recipe is special—it runs when you type `just` without an argument.
default:
  just --list

# The `test` recipe runs tests.
# You can use an `@` to suppress the command from being printed to the console.
test:
  @echo "Running tests..."
  go test ./...

# The `build` recipe builds the Go executable.
build:
  @echo "Building the project..."
  go build -o myapp .

# The `run` recipe runs the built application.
# It depends on the `build` recipe, so `just` will run `build` first.
run: build
  @echo "Running the application..."
  ./myapp

# The `clean` recipe removes the build artifacts.
clean:
  @echo "Cleaning up..."
  rm myapp
