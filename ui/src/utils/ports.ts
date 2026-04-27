// Well-known TCP port → service name mapping.
// Covers the ports Ding scans by default plus common extras.
export const PORT_NAMES: Record<number, string> = {
  21:    'FTP',
  22:    'SSH',
  23:    'Telnet',
  25:    'SMTP',
  53:    'DNS',
  67:    'DHCP',
  68:    'DHCP',
  80:    'HTTP',
  110:   'POP3',
  111:   'RPC',
  123:   'NTP',
  143:   'IMAP',
  161:   'SNMP',
  179:   'BGP',
  389:   'LDAP',
  443:   'HTTPS',
  445:   'SMB',
  465:   'SMTPS',
  514:   'Syslog',
  515:   'LPD',
  548:   'AFP',
  587:   'SMTP',
  631:   'IPP',
  636:   'LDAPS',
  873:   'rsync',
  993:   'IMAPS',
  995:   'POP3S',
  1080:  'SOCKS',
  1194:  'OpenVPN',
  1433:  'MSSQL',
  1521:  'Oracle',
  1883:  'MQTT',
  2049:  'NFS',
  2375:  'Docker',
  2376:  'Docker TLS',
  3000:  'Dev',
  3306:  'MySQL',
  3389:  'RDP',
  4243:  'Docker',
  5000:  'Dev',
  5432:  'Postgres',
  5900:  'VNC',
  6379:  'Redis',
  6443:  'K8s API',
  8080:  'HTTP-Alt',
  8443:  'HTTPS-Alt',
  8883:  'MQTT TLS',
  9000:  'Dev',
  9090:  'Prometheus',
  9200:  'Elasticsearch',
  10250: 'Kubelet',
  27017: 'MongoDB',
}

// Returns "NAME/PORT" if known (e.g. "SSH/22"), otherwise just the port number as string.
export function portLabel(port: number): string {
  const name = PORT_NAMES[port]
  return name ? `${name}/${port}` : String(port)
}

// Returns just the service name if known, otherwise the port number as string.
export function portName(port: number): string {
  return PORT_NAMES[port] ?? String(port)
}
