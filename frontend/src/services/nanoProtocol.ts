// Nano/Pomelo binary protocol implementation for WebSocket

const PKG_HEAD_BYTES = 4;
const MSG_FLAG_BYTES = 1;
const MSG_ROUTE_CODE_BYTES = 2;
const MSG_ROUTE_LEN_BYTES = 1;

export const PackageType = {
  HANDSHAKE: 1,
  HANDSHAKE_ACK: 2,
  HEARTBEAT: 3,
  DATA: 4,
  KICK: 5,
} as const;
export type PackageType = (typeof PackageType)[keyof typeof PackageType];

export const MessageType = {
  REQUEST: 0,
  NOTIFY: 1,
  RESPONSE: 2,
  PUSH: 3,
} as const;
export type MessageType = (typeof MessageType)[keyof typeof MessageType];

const MSG_COMPRESS_ROUTE_MASK = 0x1;
const MSG_TYPE_MASK = 0x7;

function strEncode(str: string): Uint8Array {
  const encoder = new TextEncoder();
  return encoder.encode(str);
}

function strDecode(buf: Uint8Array): string {
  const decoder = new TextDecoder();
  return decoder.decode(buf);
}

export function encodePackage(type: PackageType, body?: Uint8Array): Uint8Array {
  const length = body ? body.length : 0;
  const buffer = new Uint8Array(PKG_HEAD_BYTES + length);
  let index = 0;
  buffer[index++] = type & 0xff;
  buffer[index++] = (length >> 16) & 0xff;
  buffer[index++] = (length >> 8) & 0xff;
  buffer[index++] = length & 0xff;
  if (body) {
    buffer.set(body, index);
  }
  return buffer;
}

export function decodePackage(buffer: ArrayBuffer): { type: PackageType; body: Uint8Array }[] {
  const bytes = new Uint8Array(buffer);
  let offset = 0;
  const results: { type: PackageType; body: Uint8Array }[] = [];
  while (offset < bytes.length) {
    const type = bytes[offset++] as PackageType;
    const length = ((bytes[offset++] << 16) | (bytes[offset++] << 8) | bytes[offset++]) >>> 0;
    const body = new Uint8Array(length);
    body.set(bytes.subarray(offset, offset + length));
    offset += length;
    results.push({ type, body });
  }
  return results;
}

function msgHasId(type: MessageType): boolean {
  return type === MessageType.REQUEST || type === MessageType.RESPONSE;
}

function msgHasRoute(type: MessageType): boolean {
  return type === MessageType.REQUEST || type === MessageType.NOTIFY || type === MessageType.PUSH;
}

function calcMsgIdBytes(id: number): number {
  let len = 0;
  do {
    len += 1;
    id >>= 7;
  } while (id > 0);
  return len;
}

export function encodeMessage(
  id: number,
  type: MessageType,
  compressRoute: number,
  route: string | number,
  msg: Uint8Array
): Uint8Array {
  const idBytes = msgHasId(type) ? calcMsgIdBytes(id) : 0;
  let msgLen = MSG_FLAG_BYTES + idBytes;

  let routeBytes: Uint8Array | null = null;
  if (msgHasRoute(type)) {
    if (compressRoute) {
      if (typeof route !== 'number') {
        throw new Error('error flag for number route!');
      }
      msgLen += MSG_ROUTE_CODE_BYTES;
    } else {
      msgLen += MSG_ROUTE_LEN_BYTES;
      if (route) {
        routeBytes = strEncode(route as string);
        if (routeBytes.length > 255) {
          throw new Error('route maxlength is overflow');
        }
        msgLen += routeBytes.length;
      }
    }
  }

  if (msg) {
    msgLen += msg.length;
  }

  const buffer = new Uint8Array(msgLen);
  let offset = 0;

  buffer[offset++] = (type << 1) | (compressRoute ? 1 : 0);

  if (msgHasId(type)) {
    let remaining = id;
    do {
      let tmp = remaining % 128;
      const next = Math.floor(remaining / 128);
      if (next !== 0) {
        tmp += 128;
      }
      buffer[offset++] = tmp;
      remaining = next;
    } while (remaining !== 0);
  }

  if (msgHasRoute(type)) {
    if (compressRoute) {
      const routeNum = route as number;
      buffer[offset++] = (routeNum >> 8) & 0xff;
      buffer[offset++] = routeNum & 0xff;
    } else {
      if (routeBytes) {
        buffer[offset++] = routeBytes.length & 0xff;
        buffer.set(routeBytes, offset);
        offset += routeBytes.length;
      } else {
        buffer[offset++] = 0;
      }
    }
  }

  if (msg) {
    buffer.set(msg, offset);
    offset += msg.length;
  }

  return buffer;
}

export function decodeMessage(buffer: Uint8Array): {
  id: number;
  type: MessageType;
  compressRoute: number;
  route: string | number;
  body: Uint8Array;
} {
  const bytes = new Uint8Array(buffer);
  const bytesLen = bytes.length;
  let offset = 0;
  let id = 0;
  let route: string | number = '';

  const flag = bytes[offset++];
  const compressRoute = flag & MSG_COMPRESS_ROUTE_MASK;
  const rawType = (flag >> 1) & MSG_TYPE_MASK;
  const type = rawType as MessageType;

  if (msgHasId(rawType as MessageType)) {
    let m = 0;
    let i = 0;
    do {
      m = bytes[offset];
      id += (m & 0x7f) * Math.pow(2, 7 * i);
      offset++;
      i++;
    } while (m >= 128);
  }

  if (msgHasRoute(type)) {
    if (compressRoute) {
      route = (bytes[offset++] << 8) | bytes[offset++];
    } else {
      const routeLen = bytes[offset++];
      if (routeLen) {
        const routeBytes = bytes.subarray(offset, offset + routeLen);
        route = strDecode(routeBytes);
        offset += routeLen;
      } else {
        route = '';
      }
    }
  }

  const bodyLen = bytesLen - offset;
  const body = new Uint8Array(bodyLen);
  body.set(bytes.subarray(offset, offset + bodyLen));

  return { id, type: type as MessageType, compressRoute, route, body };
}
