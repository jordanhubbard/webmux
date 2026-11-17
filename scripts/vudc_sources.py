"""Bounded acquisition of checksum-pinned, disposable USB fixture sources."""
import datetime
import email.utils
import hashlib
import json
import socket
import time
import urllib.error
import urllib.request

LIMIT = 1 << 20


def retry_delay(headers, attempt):
    value = headers.get('Retry-After')
    if value is None:
        return attempt
    try:
        delay = float(int(value))
    except ValueError:
        try:
            date = email.utils.parsedate_to_datetime(value)
            delay = (date - datetime.datetime.now(datetime.timezone.utc)).total_seconds()
        except (ValueError, TypeError, OverflowError):
            raise RuntimeError('invalid source Retry-After') from None
    if delay > 20:
        raise RuntimeError('source Retry-After exceeds the 20-second wait bound')
    return max(0, delay)


def acquire(manifest, root, cache, *, opener=urllib.request.urlopen,
            sleep=time.sleep, clock=time.monotonic):
    """Write a receipt even on failure; only verified bytes enter the cache."""
    began = clock()
    receipt = {'passed': False, 'commit': manifest['commit'], 'budget_seconds': 90,
               'max_attempts': 3, 'files': []}
    path = root / 'source-acquisition.json'

    def save():
        path.write_text(json.dumps(receipt, indent=2) + '\n')

    save()
    try:
        cache.mkdir(parents=True, exist_ok=True)
        for name, expected in manifest['files'].items():
            url = f"https://raw.githubusercontent.com/torvalds/linux/{manifest['commit']}/drivers/usb/usbip/{name}"
            item = {'name': name, 'url': url, 'expected_sha256': expected,
                    'from_cache': False, 'attempts': []}
            receipt['files'].append(item)
            save()
            cached = cache / (manifest['commit'] + '-' + name)
            if cached.is_symlink():
                raise RuntimeError('refusing linked fixture cache entry')
            if cached.exists():
                data = cached.read_bytes()
                item['from_cache'] = True
            else:
                for attempt in range(1, 4):
                    remaining = 90 - (clock() - began)
                    if remaining <= 0:
                        raise RuntimeError('source acquisition budget exhausted')
                    observation = {'attempt': attempt}
                    item['attempts'].append(observation)
                    save()
                    try:
                        with opener(url, timeout=min(20, remaining)) as response:
                            data = response.read(LIMIT + 1)
                        observation['outcome'] = 'downloaded'
                        save()
                        break
                    except (urllib.error.URLError, TimeoutError, socket.timeout) as error:
                        status = error.code if isinstance(error, urllib.error.HTTPError) else None
                        headers = error.headers if status else {}
                        if status:
                            error.close()
                        observation.update(outcome='transport-error', http_status=status,
                                           error_type=type(error).__name__)
                        save()
                        if status is not None and status not in (429, 500, 502, 503, 504):
                            raise
                        if attempt == 3:
                            raise
                        delay = retry_delay(headers, attempt)
                        if delay >= 90 - (clock() - began):
                            raise RuntimeError('source retry exceeds remaining acquisition budget') from error
                        observation['retry_seconds'] = delay
                        save()
                        sleep(delay)
            if len(data) > LIMIT or hashlib.sha256(data).hexdigest() != expected:
                raise RuntimeError('kernel fixture source checksum/size mismatch: ' + name)
            item['verified_sha256'] = expected
            if not item['from_cache']:
                # Each qualification owns this cache; exclusive creation also
                # refuses an unexpected concurrent writer rather than replacing it.
                with cached.open('xb') as stream:
                    stream.write(data)
            (root / name).write_bytes(data)
            save()
        receipt['passed'] = True
    except Exception as error:
        receipt['error_type'] = type(error).__name__
        raise
    finally:
        receipt['elapsed_seconds'] = round(clock() - began, 3)
        save()
