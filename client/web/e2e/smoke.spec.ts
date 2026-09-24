import { expect, test, type Page } from '@playwright/test';

// fail on anything the app logs as an error
function watchErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text());
  });
  return errors;
}

test('fleet, host, zoom, processes at a time, alerts', async ({ page }) => {
  const errors = watchErrors(page);

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Hosts' })).toBeVisible();
  const firstHost = page.locator('a.host').first();
  await expect(firstHost).toBeVisible();
  const hostName = (await firstHost.locator('.name').textContent())!.trim();

  await firstHost.click();
  await expect(page).toHaveURL(new RegExp(`/hosts/${encodeURIComponent(hostName)}`));
  await expect(page.getByRole('heading', { name: hostName, level: 1 })).toBeVisible();
  const cpuChart = page.locator('figure', { hasText: 'CPU usage' }).first().locator('.u-over');
  await expect(cpuChart).toBeVisible();

  // drag across the chart to zoom into that window
  const box = (await cpuChart.boundingBox())!;
  await page.mouse.move(box.x + box.width * 0.3, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.7, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(page).toHaveURL(/from=\d+&to=\d+/);
  await expect(page.getByRole('button', { name: 'Custom', pressed: true })).toBeVisible();

  // click a point to see the processes at that time
  await cpuChart.click({ position: { x: box.width * 0.5, y: box.height / 2 } });
  await expect(page.locator('#processes')).toContainText('At ');
  await page.getByRole('button', { name: 'Show latest' }).click();
  await expect(page.locator('#processes')).toContainText('Latest');

  await page.getByRole('link', { name: 'Alerts' }).click();
  await expect(page.getByRole('heading', { name: 'Alerts' })).toBeVisible();
  await page.getByRole('button', { name: 'All', exact: true }).click();
  await expect(page).toHaveURL(/show=all/);

  expect(errors).toEqual([]);
});

test('theme can be picked and is remembered', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Theme').selectOption('dark');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
});

test('unknown pages and hosts are handled', async ({ page }) => {
  await page.goto('/nowhere');
  await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
  await page.goto('/hosts/no-such-host');
  await expect(page.getByText('This host has not sent any data yet.')).toBeVisible();
});

test('containers show on a host that runs them', async ({ page, request }) => {
  const fleet = await (await request.get('/api/v1/fleet')).json();
  const host = fleet.hosts.find((h: { containers: number }) => h.containers > 0);
  test.skip(!host, 'no host reports containers');

  await page.goto(`/hosts/${encodeURIComponent(host.name)}`);
  const table = page.locator('table', { has: page.locator('caption', { hasText: 'Running containers' }) });
  await expect(table).toBeVisible();
  await expect(table.locator('tbody tr')).not.toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Container CPU, share of the host' })).toBeVisible();
});
