# services.py
import time
import threading
import requests
from config import HEADERS, DISCOGS_TOKEN, EMBY_BASE_URL, EMBY_API_KEY
from database import save_discogs_to_db

mb_lock = threading.Lock()
session = requests.Session()


def fetch_wiki_summary(title):
    clean_title = requests.utils.quote(title.replace(" ", "_"))
    url = f"https://en.wikipedia.org/api/rest_v1/page/summary/{clean_title}"
    try:
        res = session.get(url, headers=HEADERS, timeout=5)
        return res.json().get('extract', '') if res.status_code == 200 else ""
    except Exception:
        return ""


def fetch_discogs_data(artist, album):
    if not DISCOGS_TOKEN:
        print("DEBUG: Discogs Token is missing!")
        return

    search_url = "https://api.discogs.com/database/search"
    params = {"artist": artist, "release_title": album, "type": "release"}
    headers = {"Authorization": f"Discogs token={DISCOGS_TOKEN}", "User-Agent": HEADERS['User-Agent']}

    try:
        time.sleep(1.1)
        res = session.get(search_url, params=params, headers=headers, timeout=10)
        data = res.json()

        results = data.get('results')
        if not results:
            print(f"DEBUG: No Discogs results found for {artist} - {album}")
            return

        release_id = results[0]['id']
        print(f"DEBUG: Found Discogs ID {release_id} for {album}. Fetching details...")

        time.sleep(1.1)
        detail_res = session.get(f"https://api.discogs.com/releases/{release_id}", headers=headers, timeout=10)

        if detail_res.status_code == 200:
            save_discogs_to_db(artist, album, detail_res.json())
            print(f"DEBUG: Discogs data saved successfully for {album}")
        else:
            print(f"DEBUG: Failed to get release details. Status: {detail_res.status_code}")

    except Exception as e:
        print(f"DEBUG: Discogs Exception: {e}")


def get_combined_metadata(artist, album):
    with mb_lock:
        search_url = "https://musicbrainz.org/ws/2/release"
        params = {"query": f'artist:"{artist}" AND release:"{album}"', "fmt": "json"}
        try:
            time.sleep(1.1)
            response = session.get(search_url, params=params, headers=HEADERS, timeout=15)
            if response.status_code == 200:
                data = response.json()
                if data.get('releases'):
                    rel = data['releases'][0]
                    mb_id = rel.get('id')
                    a_sum = fetch_wiki_summary(artist)
                    if not a_sum:
                        a_sum = fetch_wiki_summary(f"{artist} (band)")
                    l_sum = fetch_wiki_summary(f"{album} ({artist} album)")
                    if not l_sum:
                        l_sum = fetch_wiki_summary(album)

                    return {
                        "year": rel.get('date', 'Unknown'),
                        "country": rel.get('country', 'Unknown'),
                        "label": rel.get('label-info', [{}])[0].get('label', {}).get('name', 'Unknown'),
                        "mbid": mb_id,
                        "artist_summary": a_sum,
                        "album_summary": l_sum
                    }
        except Exception as e:
            print(f"MB Error: {e}")
    return None


def get_emby_server_id():
    """Fetches the unique ServerId required for deep linking."""
    url = f"{EMBY_BASE_URL}/emby/System/Info"
    params = {"api_key": EMBY_API_KEY}

    try:
        response = session.get(url, params=params, timeout=5)
        if response.status_code == 200:
            return response.json().get('Id')
    except Exception as e:
        print(f"DEBUG: Could not fetch Emby ServerId: {e}")
    return None


def get_emby_artist_id(artist_name):
    """
    Retrieves the internal Emby ID for a specific artist.
    Tries a direct match first, then falls back to a filtered search.
    """
    # 1. Try Direct Artist Lookup (Best for Artist Pages)
    encoded_name = requests.utils.quote(artist_name)
    direct_url = f"{EMBY_BASE_URL}/emby/Artists/{encoded_name}"
    params = {"api_key": EMBY_API_KEY}

    try:
        response = session.get(direct_url, params=params, timeout=5)
        if response.status_code == 200:
            artist_data = response.json()
            artist_id = artist_data.get('Id')
            if artist_id:
                print(f"DEBUG: Emby Direct Match found for '{artist_name}' (ID: {artist_id})")
                return artist_id
    except Exception as e:
        print(f"DEBUG: Emby Direct Match failed for '{artist_name}': {e}")

    # 2. Fallback: Filtered Search (In case the name format varies slightly)
    search_url = f"{EMBY_BASE_URL}/emby/Items"
    search_params = {
        "SearchTerm": artist_name,
        "IncludeItemTypes": "MusicArtist",
        "Recursive": "true",
        "api_key": EMBY_API_KEY
    }

    try:
        search_response = session.get(search_url, params=search_params, timeout=5)
        if search_response.status_code == 200:
            items = search_response.json().get('Items', [])
            if items:
                artist_id = items[0].get('Id')
                print(f"DEBUG: Emby Search Match found for '{artist_name}' (ID: {artist_id})")
                return artist_id
    except Exception as e:
        print(f"DEBUG: Emby Search Fallback failed: {e}")

    print(f"DEBUG: Emby could not find any artist matching '{artist_name}'")
    return None
