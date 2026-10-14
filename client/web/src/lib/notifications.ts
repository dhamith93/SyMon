import type { Channel, ChannelInput, ChannelKind, Notification } from './api';

// One setting of a kind of channel, as the form shows it. Secrets are never
// sent back, so when editing they stay empty and an empty one keeps the
// saved value.
export interface Field {
  key: string;
  label: string;
  type: 'text' | 'password' | 'number' | 'list';
  placeholder?: string;
  hint?: string;
  required?: boolean;
  secret?: boolean;
}

export const kindNames: Record<ChannelKind, string> = {
  email: 'Email',
  slack: 'Slack',
  pagerduty: 'PagerDuty',
  webhook: 'Webhook',
};

export const kinds = Object.keys(kindNames) as ChannelKind[];

export const kindFields: Record<ChannelKind, Field[]> = {
  email: [
    { key: 'host', label: 'SMTP server', type: 'text', placeholder: 'smtp.example.com', required: true },
    { key: 'port', label: 'Port', type: 'number', placeholder: '587', hint: '587 switches to TLS, 465 uses TLS from the start' },
    { key: 'user', label: 'User', type: 'text' },
    { key: 'password', label: 'Password', type: 'password', secret: true },
    { key: 'from', label: 'From', type: 'text', placeholder: 'SyMon <symon@example.com>', required: true },
    { key: 'to', label: 'To', type: 'list', placeholder: 'ops@example.com, oncall@example.com', hint: 'Commas between addresses', required: true },
  ],
  slack: [
    {
      key: 'token',
      label: 'Bot token',
      type: 'password',
      placeholder: 'xoxb-…',
      hint: 'A Slack app with the chat:write scope, invited to the channel',
      required: true,
      secret: true,
    },
    { key: 'channel', label: 'Channel', type: 'text', placeholder: '#ops', required: true },
  ],
  pagerduty: [
    {
      key: 'routingKey',
      label: 'Integration key',
      type: 'password',
      hint: 'From an Events API v2 integration on a PagerDuty service',
      required: true,
      secret: true,
    },
  ],
  webhook: [
    {
      key: 'url',
      label: 'URL',
      type: 'password',
      placeholder: 'https://hooks.example.com/symon',
      hint: 'Gets every alert as JSON in a POST',
      required: true,
      secret: true,
    },
    { key: 'headerName', label: 'Header name', type: 'text', placeholder: 'Authorization' },
    { key: 'headerValue', label: 'Header value', type: 'password', placeholder: 'Bearer …', secret: true },
  ],
};

// The form keeps every value as text, the way inputs hold it
export interface ChannelForm {
  name: string;
  kind: ChannelKind;
  values: Record<string, string>;
  // the secrets already saved, which an empty field keeps
  saved: string[];
  isDefault: boolean;
  skipWarnings: boolean;
  enabled: boolean;
}

export function newChannelForm(kind: ChannelKind = 'email'): ChannelForm {
  return { name: '', kind, values: {}, saved: [], isDefault: false, skipWarnings: false, enabled: true };
}

export function formFromChannel(channel: Channel): ChannelForm {
  const values: Record<string, string> = {};
  for (const field of kindFields[channel.kind]) {
    const value = channel.settings?.[field.key];
    if (Array.isArray(value)) values[field.key] = value.join(', ');
    else if (value !== undefined && value !== null && value !== '' && value !== 0) values[field.key] = String(value);
  }
  return {
    name: channel.name,
    kind: channel.kind,
    values,
    saved: channel.secretsSet ?? [],
    isDefault: channel.isDefault,
    skipWarnings: channel.skipWarnings,
    enabled: channel.enabled,
  };
}

// inputFromForm turns the text back into settings. Fields left empty are
// sent empty, which clears them, or for secrets keeps the saved one.
export function inputFromForm(form: ChannelForm): ChannelInput {
  const settings: Record<string, unknown> = {};
  for (const field of kindFields[form.kind]) {
    // a number input hands back a number, or null when it is empty
    const raw = String(form.values[field.key] ?? '');
    if (field.type === 'list') {
      settings[field.key] = raw
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
    } else if (field.type === 'number') {
      const number = Number(raw.trim());
      if (raw.trim() !== '' && Number.isFinite(number)) settings[field.key] = Math.round(number);
    } else {
      settings[field.key] = field.secret ? raw : raw.trim();
    }
  }
  return {
    name: form.name.trim(),
    kind: form.kind,
    settings,
    isDefault: form.isDefault,
    skipWarnings: form.skipWarnings,
    enabled: form.enabled,
  };
}

// what a channel sends to, for admins, who get its settings
export function describeChannel(channel: Channel): string {
  const settings = channel.settings ?? {};
  switch (channel.kind) {
    case 'email': {
      const to = Array.isArray(settings.to) ? settings.to.join(', ') : '';
      return `${to} through ${settings.host ?? ''}`;
    }
    case 'slack':
      return `${settings.channel ?? ''} on Slack`;
    case 'pagerduty':
      return 'incidents on a PagerDuty service';
    case 'webhook':
      return channel.secretsSet?.includes('url') ? 'a saved URL' : 'no URL yet';
  }
}

// which alerts a channel gets
export function describeReach(channel: Channel): string {
  if (!channel.enabled) return 'Nothing, it is off';
  const which = channel.skipWarnings ? 'Critical alerts' : 'Warnings and critical alerts';
  return channel.isDefault ? `${which}, also from rules that pick no channel` : `${which} from rules that pick it`;
}

// how a notification stands, as a badge shows it. pending with an error is
// a notification waiting for its next try.
export function deliveryState(status: string, lastError?: string): { tone: 'good' | 'bad' | 'warning' | 'busy'; label: string } {
  switch (status) {
    case 'sent':
      return { tone: 'good', label: 'Sent' };
    case 'failed':
      return { tone: 'bad', label: 'Failed' };
    case 'pending':
      return lastError ? { tone: 'warning', label: 'Trying again' } : { tone: 'busy', label: 'Sending' };
  }
  return { tone: 'busy', label: 'None yet' };
}

export function eventName(n: Pick<Notification, 'event'>): string {
  return { opened: 'Opened', changed: 'Changed', resolved: 'Resolved' }[n.event] ?? n.event;
}
