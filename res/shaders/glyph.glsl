//shader:vertex
#version 410

//aVertPos must be in the range [0,1]
layout(location=0) in vec3 aVertPos;

//Instanced
layout(location=1) in vec2 aUV0;
layout(location=2) in vec2 aUVSize;
layout(location=3) in vec4 aVertColor;
layout(location=4) in vec3 aModelPos;
layout(location=5) in vec2 aModelScale;

out vec2 v2fUV0;
out vec4 v2fColor;
out vec3 v2fModelPos;

uniform mat4 projViewMat;

void main()
{
    mat4 modelMat = mat4(
        aModelScale.x,  0.0,            0.0,            0.0, 
        0.0,            aModelScale.y,  0.0,            0.0, 
        0.0,            0.0,            1.0,            0.0, 
        aModelPos.x,    aModelPos.y,    aModelPos.z,    1.0
    );

    v2fColor = aVertColor;
    v2fModelPos = aModelPos;
    v2fUV0 = aUV0 + aVertPos.xy * aUVSize;

    gl_Position = projViewMat * modelMat * vec4(aVertPos, 1.0);
}

//shader:fragment
#version 410

in vec2 v2fUV0;
in vec4 v2fColor;
in vec3 v2fModelPos;

out vec4 fragColor;

uniform uint opts1;
uniform sampler2D diffTex;

const uint opts1_bgColorMask = 1<<0;
const uint opts1_underlineMask = 1<<1;

bool hasOpts(uint mask)
{
    return (opts1&mask) != 0;
}

void main()
{
    if (hasOpts(opts1_bgColorMask) && v2fUV0 == vec2(-1, -1))
    {
        fragColor = v2fColor;
        return;
    }

    vec4 texColor = texelFetch(diffTex, ivec2(v2fUV0), 0);
    fragColor = vec4(v2fColor.rgb, texColor.r*v2fColor.a);
}
