import { describe, expect, it } from 'vitest'

import {
  applyStorageTarget,
  imageNeedsOwnTarget,
  sameStorageTarget,
  storageTargetConfigured,
} from '../backupObjectStorage'

const target = (overrides: Partial<Parameters<typeof sameStorageTarget>[0]> = {}) => ({
  reuse_backup_s3: false,
  bucket: 'media',
  endpoint: 'https://acct.r2.cloudflarestorage.com',
  region: 'auto',
  access_key_id: 'ak',
  force_path_style: true,
  ...overrides,
})

describe('sameStorageTarget', () => {
  it('treats identical targets as shared', () => {
    expect(sameStorageTarget(target(), target())).toBe(true)
  })

  it('treats an empty region as auto', () => {
    expect(sameStorageTarget(target({ region: '' }), target({ region: 'auto' }))).toBe(true)
  })

  it('ignores surrounding whitespace', () => {
    expect(sameStorageTarget(target({ bucket: ' media ' }), target())).toBe(true)
  })

  it('only compares the bucket when both reuse the backup credentials', () => {
    const a = target({ reuse_backup_s3: true, endpoint: 'https://a.example', access_key_id: 'a' })
    const b = target({ reuse_backup_s3: true, endpoint: 'https://b.example', access_key_id: 'b' })
    expect(sameStorageTarget(a, b)).toBe(true)
    expect(sameStorageTarget(a, { ...b, bucket: 'other' })).toBe(false)
  })

  it.each([
    ['bucket', { bucket: 'other' }],
    ['endpoint', { endpoint: 'https://other.example' }],
    ['region', { region: 'us-east-1' }],
    ['access key', { access_key_id: 'other' }],
    ['path style', { force_path_style: false }],
    ['reuse flag', { reuse_backup_s3: true }],
  ])('keeps targets separate when the %s differs', (_label, overrides) => {
    expect(sameStorageTarget(target(), target(overrides))).toBe(false)
  })
})

describe('storageTargetConfigured', () => {
  const blank = target({ bucket: '', endpoint: '', access_key_id: '' })

  it('is false for a never-configured target', () => {
    expect(storageTargetConfigured(blank, false)).toBe(false)
  })

  it('is true once a secret exists even with every visible field blank', () => {
    expect(storageTargetConfigured(blank, true)).toBe(true)
  })

  it.each([['bucket'], ['endpoint'], ['access_key_id']] as const)(
    'is true once %s is filled in',
    field => {
      expect(storageTargetConfigured({ ...blank, [field]: 'x' }, false)).toBe(true)
    },
  )
})

describe('imageNeedsOwnTarget', () => {
  it('follows the shared target on a fresh install', () => {
    // Image defaults differ from video defaults (reuse_backup_s3 true vs false),
    // so a naive comparison would wrongly split an untouched install.
    const image = target({ reuse_backup_s3: true, bucket: '', endpoint: '', access_key_id: '' })
    expect(imageNeedsOwnTarget(image, false, target())).toBe(false)
  })

  it('keeps a configured image target that points somewhere else', () => {
    expect(imageNeedsOwnTarget(target({ bucket: 'images' }), true, target())).toBe(true)
  })

  it('shares when a configured image target matches', () => {
    expect(imageNeedsOwnTarget(target(), true, target())).toBe(false)
  })
})

describe('applyStorageTarget', () => {
  it('copies only the target fields and blanks the secret by default', () => {
    const image = {
      ...target({ bucket: 'images', endpoint: 'https://old.example' }),
      prefix: 'images/',
      public_base_url: 'https://cdn.example',
      presign_expiry_hours: 24,
      max_download_bytes: 33554432,
      secret_access_key: 'stale',
    }
    const merged = applyStorageTarget(image, target())

    expect(merged.bucket).toBe('media')
    expect(merged.endpoint).toBe('https://acct.r2.cloudflarestorage.com')
    // Untouched: these belong to images, not to the shared target.
    expect(merged.prefix).toBe('images/')
    expect(merged.public_base_url).toBe('https://cdn.example')
    expect(merged.max_download_bytes).toBe(33554432)
    // Empty means "keep whatever is stored", the contract both backends use.
    expect(merged.secret_access_key).toBe('')
  })

  it('propagates a freshly typed shared secret', () => {
    const merged = applyStorageTarget(
      { ...target(), secret_access_key: '' },
      { ...target(), secret_access_key: 'typed-now' },
    )
    expect(merged.secret_access_key).toBe('typed-now')
  })
})
