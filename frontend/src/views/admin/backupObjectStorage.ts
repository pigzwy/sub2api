/**
 * 对象存储卡片把「异步生图存储」和「异步媒体存储」两份后端配置渲染成一张卡。
 *
 * 后端仍是两个独立的 settings key（image_storage_config / video_storage_config），
 * 没有迁移。卡片默认让三种类型共用同一个存储目标，保存时把共用目标写进两份配置；
 * 只有当图片确实指向别的桶时才展开它自己的目标块。
 */

/** 后端两份配置共有的存储目标字段（Secret 永不回传，故不参与比较）。 */
export interface StorageTarget {
  reuse_backup_s3: boolean
  bucket: string
  endpoint: string
  region: string
  access_key_id: string
  force_path_style: boolean
}

const trim = (value: string | undefined): string => (value ?? '').trim()

/** region 留空等价于 auto（后端 newS3Client 的默认值）。 */
const normalizedRegion = (value: string | undefined): string => trim(value) || 'auto'

/**
 * 判断两份配置是否指向同一个存储目标。
 *
 * Secret 不参与比较——接口从不回传它，只能靠可见字段判断。因此这里刻意保守：
 * 任何一项对不上就算作不同目标，宁可多展开一个目标块，也不能悄悄把图片改指到
 * 另一个桶。
 */
export function sameStorageTarget(a: StorageTarget, b: StorageTarget): boolean {
  if (a.reuse_backup_s3 !== b.reuse_backup_s3) return false
  if (trim(a.bucket) !== trim(b.bucket)) return false
  // 复用备份凭证时，endpoint/region/密钥都来自备份卡，比较桶名就够了。
  if (a.reuse_backup_s3) return true
  return (
    trim(a.endpoint) === trim(b.endpoint) &&
    normalizedRegion(a.region) === normalizedRegion(b.region) &&
    trim(a.access_key_id) === trim(b.access_key_id) &&
    a.force_path_style === b.force_path_style
  )
}

/**
 * 该配置是否被真正配置过。全新安装时图片配置是空的，应当直接跟随共用目标，而不是
 * 因为默认值不同就被判成「独立目标」。
 */
export function storageTargetConfigured(target: StorageTarget, secretConfigured: boolean): boolean {
  return (
    secretConfigured ||
    trim(target.bucket) !== '' ||
    trim(target.endpoint) !== '' ||
    trim(target.access_key_id) !== ''
  )
}

/**
 * 加载后判断图片是否需要保留独立的存储目标块。
 */
export function imageNeedsOwnTarget(
  image: StorageTarget,
  imageSecretConfigured: boolean,
  media: StorageTarget,
): boolean {
  if (!storageTargetConfigured(image, imageSecretConfigured)) return false
  return !sameStorageTarget(image, media)
}

/** 把共用目标复制到目标配置上，其余字段（前缀、公开域名等）原样保留。 */
export function applyStorageTarget<T extends StorageTarget & { secret_access_key?: string }>(
  destination: T,
  source: StorageTarget & { secret_access_key?: string },
): T {
  return {
    ...destination,
    reuse_backup_s3: source.reuse_backup_s3,
    bucket: source.bucket,
    endpoint: source.endpoint,
    region: source.region,
    access_key_id: source.access_key_id,
    force_path_style: source.force_path_style,
    // 留空表示沿用各自已存的 Secret，两个后端服务都是这个约定。
    secret_access_key: source.secret_access_key ?? '',
  }
}
