const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await page.goto(process.argv[2]);
  console.log(await page.evaluate(() => document.title));
  await browser.close();
})();
