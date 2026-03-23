const http = require('http');

async function testLogin(identifier) {
  const data = JSON.stringify({
    loginIdentifier: identifier,
    password: "password123",
    clientId: "555798ba-c23b-4757-9cd3-8944de52df81"
  });

  const req = http.request({
    hostname: 'localhost',
    port: 28191,
    path: '/api/auth/login',
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Content-Length': data.length
    }
  }, (res) => {
    let body = '';
    res.on('data', chunk => body += chunk);
    res.on('end', () => console.log(`[${identifier}] Status: ${res.statusCode} | Body: ${body.substring(0, 100)}`));
  });

  req.write(data);
  req.end();
}

setTimeout(() => testLogin("admin@example.com"), 0);
setTimeout(() => testLogin("admin"), 1000);
