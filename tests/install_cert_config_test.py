"""Exercise the installer's actual config generator without network/root changes."""
import json
import os
import pathlib
import subprocess
import sys
import tempfile

source = (pathlib.Path(__file__).resolve().parents[1] / 'install-custom.sh').read_text()
generator = source.split("<<'PY'\n", 1)[1].split('\nPY\n', 1)[0]
with tempfile.TemporaryDirectory() as tmp:
    output = pathlib.Path(tmp) / 'config.yml'
    for domain in ['', 'tuic.example.com']:
        env = dict(os.environ, PANEL_URL='https://panel.example.com', MACHINE_ID='17',
                   MACHINE_TOKEN='fixture-token', CERT_DOMAIN=domain)
        subprocess.run([sys.executable, '-', str(output)], input=generator,
                       text=True, env=env, check=True)
        config = json.loads(output.read_text())
        assert config['machine']['machine_id'] == 17
        if domain:
            assert config['cert'] == {
                'cert_mode': 'file',
                'cert_file': '/etc/letsencrypt/live/tuic.example.com/fullchain.pem',
                'key_file': '/etc/letsencrypt/live/tuic.example.com/privkey.pem',
            }
        else:
            assert 'cert' not in config
        print('certificate enabled' if domain else 'certificate skipped', 'PASS')
