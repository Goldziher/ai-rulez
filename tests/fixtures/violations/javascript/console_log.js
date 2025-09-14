// Test file with intentional console.log violations
// This file is used to test enforcement detection capabilities

function processUserData(userData) {
    // Violation: Using console.log instead of proper logging
    console.log("Starting to process user data");
    console.log("User ID:", userData.id);

    // Violation: No error handling for async operation
    fetch('/api/users/' + userData.id)
        .then(response => response.json())
        .then(data => {
            console.log("Received data:", data);
            return data;
        });

    // Violation: Magic number without constant
    if (userData.age > 18) {
        console.log("User is an adult");
    }

    // Violation: No documentation for function
    function validateEmail(email) {
        // Violation: Complex regex as magic string
        const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        return emailRegex.test(email);
    }

    // Violation: Deeply nested code
    if (userData) {
        if (userData.profile) {
            if (userData.profile.settings) {
                if (userData.profile.settings.notifications) {
                    console.log("Notifications enabled");
                }
            }
        }
    }

    // Violation: Function too long (imagine this continues for 60+ lines)
    function processEverything() {
        console.log("Step 1");
        console.log("Step 2");
        console.log("Step 3");
        // ... many more steps
    }
}

// Violation: Global variable
var GLOBAL_CONFIG = {
    debug: true
};

// Violation: Using alert in production code
function showError(message) {
    alert("Error: " + message);
    console.error(message);
}