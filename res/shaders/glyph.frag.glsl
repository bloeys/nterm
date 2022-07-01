#version 410

uniform sampler2D diffTex;

in vec4 vertColor;
in vec2 vertUV0;
in vec3 fragPos;

out vec4 fragColor;

void main()
{
    vec4 texColor = texture(diffTex, vertUV0);
    fragColor = vec4(vertColor.rgb, texColor.r*texColor.a);
} 
