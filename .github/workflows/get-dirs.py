import json
import glob
import re


def get_dirs():
    """
    Get all directories containing Lambda function Go code;
    returning a string formatted json array of the example directories minus
    those that are excluded
    """
    exclude = {
        #'lambda/example',  # Add examples here to exclude from tests
    }

    projects = {
        x.replace('/main.go', '')
        for x in glob.glob('lambda/**/main.go', recursive=True)
        if not re.match(r'^.+/_', x)
    }

    print(json.dumps(list(projects.difference(exclude))))


if __name__ == '__main__':
    get_dirs()
