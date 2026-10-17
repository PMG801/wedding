import { describe, expect, it } from 'vitest'
import { isVideoFile, isVideoMime } from './kind'

describe('media kind', () => {
  it('recognizes videos by type, and by extension only when the type is empty', () => {
    expect(isVideoFile({ name: 'a.mp4', type: 'video/mp4' })).toBe(true)
    expect(isVideoFile({ name: 'a.mov', type: 'video/quicktime' })).toBe(true)
    expect(isVideoFile({ name: 'A.MOV', type: '' })).toBe(true)
    expect(isVideoFile({ name: 'a.jpg', type: 'image/jpeg' })).toBe(false)
    expect(isVideoFile({ name: 'a.mp4', type: 'image/jpeg' })).toBe(false)
    expect(isVideoMime('video/mp4')).toBe(true)
    expect(isVideoMime('image/heic')).toBe(false)
  })
})
