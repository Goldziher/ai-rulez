"""Test file with print statement violations for Python"""

def process_data(data):
    # Violation: Using print instead of logging
    print("Processing data...")
    print(f"Data length: {len(data)}")

    # Violation: No error handling
    result = data[0] / data[1]

    # Violation: Magic number
    if len(data) > 100:
        print("Large dataset")

    # Violation: Bare except clause
    try:
        import some_module
    except:
        print("Failed to import")

    return result


class DataProcessor:
    def __init__(self):
        # Violation: Print in constructor
        print("Initializing DataProcessor")

    def process(self, items):
        # Violation: No type hints
        for item in items:
            print(f"Processing: {item}")

        # Violation: Global variable modification
        global COUNTER
        COUNTER += 1


# Violation: Global variable
COUNTER = 0


def divide_numbers(a, b):
    # Violation: No zero division check
    return a / b


def parse_json(json_string):
    # Violation: No error handling for JSON parsing
    import json
    return json.loads(json_string)


# Violation: Function without docstring
def mysterious_function(x, y, z):
    print(f"Computing {x} + {y} * {z}")
    return x + y * z


# Violation: Catching too broad exception
def risky_operation():
    try:
        # Some risky code
        result = 10 / 0
    except Exception as e:
        print(f"Something went wrong: {e}")


# Violation: Using eval (security risk)
def execute_string(code_string):
    print(f"Executing: {code_string}")
    return eval(code_string)