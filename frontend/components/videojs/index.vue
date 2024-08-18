<script setup lang="ts">
import {VideoPlayer} from "@videojs-player/vue";

const {video, poster} = defineProps({
  video: String,
  poster: String,
})
</script>

<template>
  <video-player
      :control-bar="{
         skipButtons:{
          forward: 10,
          backward: 30
        }
       }"
      :volume="1"
      :responsive="true"
      :preferFullWindow="true"
      :playsinline="true"
      :playbackRates="[0.5, 1,1.25, 1.5, 2]"
      :nativeControlsForTouch="true"
      class="w-100 h-100 rounded d-block"
      controls
      preload="auto"
      :src="video"
      :poster="poster"
  />
</template>