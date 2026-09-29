import { describe, expect, it } from 'vitest';
import type { RuleConfig } from './api';
import { applyKindDefaults, describeHosts, describeLevels, describeWatch, formFromRule, newForm, notifies, ruleFromForm } from './rules';

// alerts.json entries as the collector sends them back
const cpu: RuleConfig = {
  Name: 'CPU usage',
  MetricName: 'procUsage',
  Op: '>',
  WarnThreshold: 80,
  CriticalThreshold: 95,
  TriggerIntveral: 120,
  Servers: ['*'],
  Template: '{subject}',
  Email: false,
  Slack: true,
  Pagerduty: false,
};
const shop: RuleConfig = {
  Name: 'Shop',
  MetricName: 'endpoint',
  Endpoint: 'https://shop.example.com',
  Method: 'GET',
  ExpectedHTTPCode: 200,
  TriggerIntveral: 120,
  Servers: null,
  Template: '{subject}',
  CertWarnDays: 30,
};

describe('describing rules', () => {
  it('says what a rule watches and when it alerts', () => {
    expect(describeWatch(cpu)).toBe('CPU usage');
    expect(describeHosts(cpu)).toBe('All hosts');
    expect(describeLevels(cpu)).toBe('warning > 80%, critical > 95%');
    expect(notifies(cpu)).toBe('Slack');
    expect(describeWatch(shop)).toBe('GET https://shop.example.com');
    expect(describeHosts(shop)).toBe('Checked from the collector');
    expect(describeLevels(shop)).toBe('critical when not HTTP 200, certificate warning under 30 days, critical under 3');
    expect(notifies(shop)).toBe('dashboard only');
    expect(describeLevels({ Name: 'x', MetricName: 'disk_forecast', Op: '<', WarnThreshold: 14, CriticalThreshold: 3, Disk: '/dev/sda1' })).toBe(
      'warning < 14 days, critical < 3 days',
    );
    expect(describeLevels({ Name: 'x', MetricName: 'ping', TriggerIntveral: 300 })).toBe('critical after 5m of silence');
  });
});

describe('the rule form', () => {
  it('gives a rule back unchanged', () => {
    expect(ruleFromForm(formFromRule(cpu, true))).toEqual(cpu);
    expect(ruleFromForm(formFromRule(shop, true))).toEqual({ ...shop, Servers: undefined, Email: false, Slack: false, Pagerduty: false });
  });

  it('keeps only the fields a kind uses', () => {
    const form = newForm();
    form.name = 'Data disk';
    applyKindDefaults(form, 'disks');
    form.disk = ' /dev/sdb1 ';
    form.allHosts = false;
    form.hosts = ['web1'];
    form.slackChannel = '#ops';
    expect(ruleFromForm(form)).toEqual({
      Name: 'Data disk',
      MetricName: 'disks',
      Op: '>',
      WarnThreshold: 80,
      CriticalThreshold: 90,
      TriggerIntveral: 120,
      Servers: ['web1'],
      Disk: '/dev/sdb1',
      Template: '{subject}\n{serverName} {metricName} {value} {op} {expected} at {timestamp}',
      Email: false,
      Slack: false,
      Pagerduty: false,
    });
  });

  it('brings the usual levels when the kind changes', () => {
    const form = newForm();
    applyKindDefaults(form, 'disk_forecast');
    expect([form.op, form.warn, form.critical]).toEqual(['<', 14, 3]);
    applyKindDefaults(form, 'ping');
    expect(form.trigger).toBe(120);
    expect(form.template).toContain('has sent nothing');
    const custom = newForm();
    applyKindDefaults(custom, 'custom');
    custom.customName = 'queue';
    expect(ruleFromForm(custom)).toMatchObject({ MetricName: 'queue', IsCustom: true, Servers: ['*'] });
  });
});
