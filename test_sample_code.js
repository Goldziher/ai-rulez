// Sample JavaScript file for testing enforcement
function calculateTotal(items) {
    console.log("Calculating total for items:", items); // This should trigger no-console-log rule

    let total = 0;
    for (let i = 0; i < items.length; i++) {
        total += items[i].price; // No error handling - should trigger proper-error-handling rule
    }

    return total;
}

// Function with poor naming and no comments - should trigger coding-standards
function x(a, b) {
    return a + b;
}

// Function that might throw but no error handling
function processData(data) {
    return JSON.parse(data); // Could throw, no try-catch
}