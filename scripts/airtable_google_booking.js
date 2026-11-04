// Airtable Run script input: recordId = trigger record's Airtable record ID.
// Reuse the existing protected secret named exe-webhookAuthKey.
const { recordId } = input.config();
const key = input.secret('exe-webhookAuthKey');
if (!recordId) throw new Error('Map the trigger Airtable record ID to recordId.');

let response;
let failure;
for (let attempt = 1; attempt <= 3; attempt++) {
  try {
    response = await fetch('https://tistheseasonkc.com/ads/google/booked', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${key}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ recordId }),
    });
    if (response.ok || response.status < 500) break;
    failure = `Uploader returned HTTP ${response.status}`;
  } catch (_) {
    failure = 'Could not reach booking uploader';
  }
}
if (!response || !response.ok) {
  throw new Error(failure || `Uploader returned HTTP ${response.status}`);
}
const result = await response.json();
output.set('uploadState', result.state);
output.set('sendingEnabled', result.sendingEnabled === true);
// Queued means saved durably, not counted by Google. The uploader updates
// Google Booking Upload and Google Booking Upload Details as processing runs.
