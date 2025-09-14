// Test file with missing error handling violations

async function fetchDataWithoutHandling() {
    // Violation: No try-catch around async operation
    const response = await fetch('/api/data');
    const data = await response.json();
    return data;
}

function parseJSONUnsafe(jsonString) {
    // Violation: JSON.parse without try-catch
    return JSON.parse(jsonString);
}

function divideNumbers(a, b) {
    // Violation: No check for division by zero
    return a / b;
}

async function multipleAsyncCalls() {
    // Violation: No error handling for any of these
    const user = await fetch('/api/user').then(r => r.json());
    const posts = await fetch('/api/posts').then(r => r.json());
    const comments = await fetch('/api/comments').then(r => r.json());

    return { user, posts, comments };
}

function readFileUnsafe(filename) {
    // Violation: File operations without error handling
    const fs = require('fs');
    const content = fs.readFileSync(filename, 'utf8');
    return content;
}

// Violation: Promise without .catch()
function promiseWithoutCatch() {
    doSomethingAsync()
        .then(result => {
            console.log(result);
        });
    // Missing .catch()
}

// Violation: Callback without error parameter check
function callbackWithoutErrorCheck(callback) {
    someAsyncOperation((err, data) => {
        // Not checking err parameter
        callback(data);
    });
}