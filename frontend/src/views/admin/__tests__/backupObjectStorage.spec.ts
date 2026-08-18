import { describe, expect, it } from 'vitest'

import {
  applyStorageCredentials,
  imageNeedsOwnCredentials,
  sameStorageCredentials,
  storageCredentialsConfigured,
} from '../backupObjectStorage'

const creds = (overrides: Partial<Parameters<typeof sameStorageCredentials>[0]> = {}) => ({
  reuse_backup_s3: false,
  endpoint: 'https://acct.r2.cloudflarestorage.com',
  region: 'auto',
  access_key_id: 'ak',
  force_path_style: true,
  ...overrides,
})

describe('sameStorageCredentials', () => {
  it('treats identical credentials as shared', () => {
    expect(sameStorageCredentials(creds(), creds())).toBe(true)
  })

  it('ignores the bucket entirely', () => {
    // The regression this whole regrouping fixes: one R2 account with a bucket
    // per media type must not be reported as two credential sets.
    const image = { ...creds(), bucket: 'sub2-image' }
    const media = { ...creds(), bucket: 'sub2-video' }
    expect(sameStorageCredentials(image, media)).toBe(true)
  })

  it('treats an empty region as auto', () => {
    expect(sameStorageCredentials(creds({ region: '' }), creds({ region: 'auto' }))).toBe(true)
  })

  it('ignores surrounding whitespace', () => {
    expect(sameStorageCredentials(creds({ access_key_id: ' ak ' }), creds())).toBe(true)
  })

  it('needs no further comparison when both reuse the backup credentials', () => {
    const a = creds({ reuse_backup_s3: true, endpoint: 'https://a.example', access_key_id: 'a' })
    const b = creds({ reuse_backup_s3: true, endpoint: 'https://b.example', access_key_id: 'b' })
    expect(sameStorageCredentials(a, b)).toBe(true)
  })

  it.each([
    ['endpoint', { endpoint: 'https://other.example' }],
    ['region', { region: 'us-east-1' }],
    ['access key', { access_key_id: 'other' }],
    ['path style', { force_path_style: false }],
    ['reuse flag', { reuse_backup_s3: true }],
  ])('keeps credentials separate when the %s differs', (_label, overrides) => {
    expect(sameStorageCredentials(creds(), creds(overrides))).toBe(false)
  })
})

describe('storageCredentialsConfigured', () => {
  const blank = creds({ endpoint: '', access_key_id: '' })

  it('is false for never-configured credentials', () => {
    expect(storageCredentialsConfigured(blank, false)).toBe(false)
  })

  it('is true once a secret exists even with every visible field blank', () => {
    expect(storageCredentialsConfigured(blank, true)).toBe(true)
  })

  it.each([['endpoint'], ['access_key_id']] as const)('is true once %s is filled in', field => {
    expect(storageCredentialsConfigured({ ...blank, [field]: 'x' }, false)).toBe(true)
  })
})

describe('imageNeedsOwnCredentials', () => {
  it('follows the shared credentials on a fresh install', () => {
    // Image defaults differ from video defaults (reuse_backup_s3 true vs false),
    // so a naive comparison would wrongly split an untouched install.
    const image = creds({ reuse_backup_s3: true, endpoint: '', access_key_id: '' })
    expect(imageNeedsOwnCredentials(image, false, creds())).toBe(false)
  })

  it('shares when only the bucket differs', () => {
    const image = { ...creds(), bucket: 'sub2-image' }
    expect(imageNeedsOwnCredentials(image, true, { ...creds(), bucket: 'sub2-video' })).toBe(false)
  })

  it('splits when images point at another S3 account', () => {
    expect(imageNeedsOwnCredentials(creds({ access_key_id: 'other' }), true, creds())).toBe(true)
  })
})

describe('applyStorageCredentials', () => {
  it('copies credentials but never the bucket or the type-specific fields', () => {
    const image = {
      ...creds({ endpoint: 'https://old.example', access_key_id: 'old' }),
      bucket: 'sub2-image',
      prefix: 'images/',
      public_base_url: 'https://cdn.example',
      presign_expiry_hours: 168,
      max_download_bytes: 33554432,
      secret_access_key: 'stale',
    }
    const merged = applyStorageCredentials(image, creds())

    expect(merged.endpoint).toBe('https://acct.r2.cloudflarestorage.com')
    expect(merged.access_key_id).toBe('ak')
    // Untouched: these belong to images, not to the shared credentials.
    expect(merged.bucket).toBe('sub2-image')
    expect(merged.prefix).toBe('images/')
    expect(merged.public_base_url).toBe('https://cdn.example')
    expect(merged.presign_expiry_hours).toBe(168)
    expect(merged.max_download_bytes).toBe(33554432)
    // Empty means "keep whatever is stored", the contract both backends use.
    expect(merged.secret_access_key).toBe('')
  })

  it('propagates a freshly typed shared secret', () => {
    const merged = applyStorageCredentials(
      { ...creds(), secret_access_key: '' },
      { ...creds(), secret_access_key: 'typed-now' },
    )
    expect(merged.secret_access_key).toBe('typed-now')
  })
})
