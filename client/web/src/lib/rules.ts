import type { RuleConfig } from './api';
import { formatDuration } from './format';

// What a rule watches. Custom metrics have their own name as MetricName.
export type Kind = 'procUsage' | 'memory' | 'swap' | 'disks' | 'disk_forecast' | 'services' | 'ping' | 'endpoint' | 'custom';

export const kinds: { value: Kind; label: string }[] = [
  { value: 'procUsage', label: 'CPU usage, %' },
  { value: 'memory', label: 'Memory used, %' },
  { value: 'swap', label: 'Swap used, %' },
  { value: 'disks', label: 'Disk space used, %' },
  { value: 'disk_forecast', label: 'Days until a disk is full' },
  { value: 'services', label: 'A service stops or runs' },
  { value: 'ping', label: 'Host not reporting' },
  { value: 'endpoint', label: 'HTTP endpoint' },
  { value: 'custom', label: 'Custom metric' },
];

// kinds compared with a warning and a critical level
export const thresholdKinds: Kind[] = ['procUsage', 'memory', 'swap', 'disks', 'disk_forecast', 'custom'];

export const ops = ['>', '<', '>=', '<=', '==', '!='];

export function kindOf(rule: RuleConfig): Kind {
  return rule.IsCustom ? 'custom' : (rule.MetricName as Kind);
}

function unit(kind: Kind): string {
  if (kind === 'disk_forecast') return ' days';
  return kind === 'custom' ? '' : '%';
}

// what a rule watches, in words
export function describeWatch(rule: RuleConfig): string {
  switch (kindOf(rule)) {
    case 'procUsage':
      return 'CPU usage';
    case 'memory':
      return 'Memory used';
    case 'swap':
      return 'Swap used';
    case 'disks':
      return `Disk ${rule.Disk}`;
    case 'disk_forecast':
      return `${rule.Disk} filling up`;
    case 'services':
      return `Service ${rule.Service}`;
    case 'ping':
      return 'Host not reporting';
    case 'endpoint':
      return `${rule.Method || 'GET'} ${rule.Endpoint}`;
    case 'custom':
      return `Custom metric ${rule.MetricName}`;
  }
  return rule.MetricName;
}

export function describeHosts(rule: RuleConfig): string {
  if (kindOf(rule) === 'endpoint') return 'Checked from the collector';
  const servers = rule.Servers ?? [];
  return servers.includes('*') ? 'All hosts' : servers.join(', ');
}

// when a rule alerts, in words
export function describeLevels(rule: RuleConfig): string {
  const kind = kindOf(rule);
  if (thresholdKinds.includes(kind)) {
    const u = unit(kind);
    return `warning ${rule.Op} ${rule.WarnThreshold}${u}, critical ${rule.Op} ${rule.CriticalThreshold}${u}`;
  }
  switch (kind) {
    case 'services':
      return rule.Op === 'active' ? 'critical when it runs' : 'critical when it stops';
    case 'ping':
      return `critical after ${formatDuration(rule.TriggerIntveral ?? 0)} of silence`;
    case 'endpoint': {
      const text = `critical when not HTTP ${rule.ExpectedHTTPCode || 200}`;
      if (!rule.Endpoint?.toLowerCase().startsWith('https://')) return text;
      const warn = rule.CertWarnDays ?? 14;
      const critical = rule.CertCriticalDays ?? 3;
      if (warn === 0 && critical === 0) return `${text}, certificate not checked`;
      return `${text}, certificate warning under ${warn} days, critical under ${critical}`;
    }
  }
  return '';
}

export function notifies(rule: RuleConfig): string {
  const channels = [rule.Email && 'email', rule.Slack && 'Slack', rule.Pagerduty && 'PagerDuty'].filter(Boolean);
  return channels.length > 0 ? channels.join(', ') : 'dashboard only';
}

// The editor's fields. Numbers are null while their input is empty.
export interface RuleForm {
  name: string;
  description: string;
  kind: Kind;
  enabled: boolean;
  allHosts: boolean;
  hosts: string[];
  disk: string;
  service: string;
  customName: string;
  op: string;
  warn: number | null;
  critical: number | null;
  trigger: number | null;
  endpoint: string;
  method: string;
  expected: number | null;
  postBody: string;
  postContentType: string;
  customCACert: string;
  certWarn: number | null;
  certCritical: number | null;
  email: boolean;
  slack: boolean;
  slackChannel: string;
  pagerduty: boolean;
  template: string;
}

// starting levels for a new rule of each kind
const defaults: Partial<Record<Kind, { op: string; warn: number; critical: number }>> = {
  procUsage: { op: '>', warn: 80, critical: 95 },
  memory: { op: '>', warn: 80, critical: 90 },
  swap: { op: '>', warn: 50, critical: 80 },
  disks: { op: '>', warn: 80, critical: 90 },
  disk_forecast: { op: '<', warn: 14, critical: 3 },
  custom: { op: '>', warn: 0, critical: 0 },
};

export function defaultTemplate(kind: Kind): string {
  if (kind === 'endpoint') return '{subject}\n{endpoint} expected {expected}, got {actual}: {error}';
  if (kind === 'ping') return '{subject}\n{serverName} has sent nothing for over {triggerInterval} seconds';
  return '{subject}\n{serverName} {metricName} {value} {op} {expected} at {timestamp}';
}

// switching what a rule watches brings that kind's usual levels, so a CPU
// rule turned into a disk forecast does not keep "> 80"
export function applyKindDefaults(form: RuleForm, kind: Kind) {
  const previous = form.kind;
  form.kind = kind;
  const levels = defaults[kind];
  if (levels) {
    form.op = levels.op;
    form.warn = levels.warn;
    form.critical = levels.critical;
  } else if (kind === 'services') {
    form.op = 'inactive';
  }
  if (kind === 'ping' && (form.trigger ?? 0) < 60) form.trigger = 300;
  if (form.template === '' || form.template === defaultTemplate(previous)) form.template = defaultTemplate(kind);
}

export function newForm(): RuleForm {
  const form: RuleForm = {
    name: '',
    description: '',
    kind: 'procUsage',
    enabled: true,
    allHosts: true,
    hosts: [],
    disk: '',
    service: '',
    customName: '',
    op: '>',
    warn: null,
    critical: null,
    trigger: 120,
    endpoint: '',
    method: 'GET',
    expected: 200,
    postBody: '',
    postContentType: 'application/json',
    customCACert: '',
    certWarn: null,
    certCritical: null,
    email: false,
    slack: false,
    slackChannel: '',
    pagerduty: false,
    template: '',
  };
  applyKindDefaults(form, 'procUsage');
  return form;
}

export function formFromRule(rule: RuleConfig, enabled: boolean): RuleForm {
  const servers = rule.Servers ?? [];
  return {
    name: rule.Name,
    description: rule.Description ?? '',
    kind: kindOf(rule),
    enabled,
    allHosts: servers.includes('*'),
    hosts: servers.filter((s) => s !== '*'),
    disk: rule.Disk ?? '',
    service: rule.Service ?? '',
    customName: rule.IsCustom ? rule.MetricName : '',
    op: rule.Op ?? '',
    warn: rule.WarnThreshold ?? null,
    critical: rule.CriticalThreshold ?? null,
    trigger: rule.TriggerIntveral ?? 0,
    endpoint: rule.Endpoint ?? '',
    method: rule.Method || 'GET',
    expected: rule.ExpectedHTTPCode || 200,
    postBody: rule.POSTBody ?? '',
    postContentType: rule.POSTContentType || 'application/json',
    customCACert: rule.CustomCACert ?? '',
    certWarn: rule.CertWarnDays ?? null,
    certCritical: rule.CertCriticalDays ?? null,
    email: !!rule.Email,
    slack: !!rule.Slack,
    slackChannel: rule.SlackChannel ?? '',
    pagerduty: !!rule.Pagerduty,
    template: rule.Template ?? '',
  };
}

// ruleFromForm keeps only the fields the kind uses, so a rule reads like a
// hand written alerts.json entry
export function ruleFromForm(form: RuleForm): RuleConfig {
  const kind = form.kind;
  const rule: RuleConfig = {
    Name: form.name.trim(),
    MetricName: kind === 'custom' ? form.customName.trim() : kind,
    TriggerIntveral: Math.round(form.trigger ?? 0),
    Template: form.template || defaultTemplate(kind),
    Email: form.email,
    Slack: form.slack,
    Pagerduty: form.pagerduty,
  };
  if (form.description.trim()) rule.Description = form.description.trim();
  if (form.slack && form.slackChannel.trim()) rule.SlackChannel = form.slackChannel.trim();
  if (kind === 'custom') rule.IsCustom = true;
  if (kind !== 'endpoint') rule.Servers = form.allHosts ? ['*'] : form.hosts;
  if (thresholdKinds.includes(kind)) {
    rule.Op = form.op;
    rule.WarnThreshold = Math.round(form.warn ?? 0);
    rule.CriticalThreshold = Math.round(form.critical ?? 0);
  }
  if (kind === 'disks' || kind === 'disk_forecast') rule.Disk = form.disk.trim();
  if (kind === 'services') {
    rule.Service = form.service.trim();
    rule.Op = form.op;
  }
  if (kind === 'endpoint') {
    rule.Endpoint = form.endpoint.trim();
    rule.Method = form.method;
    rule.ExpectedHTTPCode = Math.round(form.expected ?? 200);
    if (form.method === 'POST') {
      rule.POSTBody = form.postBody;
      rule.POSTContentType = form.postContentType;
    }
    if (form.customCACert.trim()) rule.CustomCACert = form.customCACert.trim();
    if (form.certWarn !== null) rule.CertWarnDays = Math.round(form.certWarn);
    if (form.certCritical !== null) rule.CertCriticalDays = Math.round(form.certCritical);
  }
  return rule;
}
