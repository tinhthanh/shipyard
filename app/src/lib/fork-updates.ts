// This fork's releases carry berth and berthd, and a signed app for Windows
// only, so on a Mac or Linux the updater's check finds no latest.json, or one
// with nothing for this platform. That isn't a failure to report: the app
// here is built from source.

export function noAppRelease(error: string): boolean {
  return /valid release JSON|latest\.json|`platforms` object/i.test(error) || /\b404\b/.test(error);
}

export const NO_APP_RELEASE_LINE = "Fork build: update the app from source";
