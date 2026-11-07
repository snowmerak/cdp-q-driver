// Run: cdp-q-driver --session research --timeout 60s run examples/scripts/extract-links.js https://example.com
// Optional second argument: an observed CSS selector for the requested links.
const page = await session.newPage("links", session.args[0] || "https://example.com");
const selector = session.args[1] || "a[href]";
await page.waitForFunction(css => document.querySelectorAll(css).length > 0, selector);
return await page.evaluate(css => [...document.querySelectorAll(css)].map(a => ({
  title: a.textContent.trim(),
  url: a.href,
})), selector);
